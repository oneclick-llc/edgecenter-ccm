package edgecenter

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// resolveServiceRegionAndFloatingIP reads the optional regional load balancer
// settings from a Service. Both annotations must be present together.
func resolveServiceRegionAndFloatingIP(service *corev1.Service, defaultRegionID int) (int, net.IP, error) {
	regionValue, hasRegion := service.Annotations[ServiceAnnotationLoadBalancerRegionID]
	floatingIPValue, hasFloatingIP := service.Annotations[ServiceAnnotationLoadBalancerFloatingIP]
	if hasRegion != hasFloatingIP {
		return 0, nil, fmt.Errorf("annotations %q and %q must be set together", ServiceAnnotationLoadBalancerRegionID, ServiceAnnotationLoadBalancerFloatingIP)
	}
	if !hasRegion {
		return defaultRegionID, nil, nil
	}

	regionID, err := strconv.Atoi(strings.TrimSpace(regionValue))
	if err != nil || regionID <= 0 {
		return 0, nil, fmt.Errorf("annotation %q must contain a positive Edgecenter region ID", ServiceAnnotationLoadBalancerRegionID)
	}

	floatingIP := net.ParseIP(strings.TrimSpace(floatingIPValue))
	if floatingIP == nil || floatingIP.To4() == nil {
		return 0, nil, fmt.Errorf("annotation %q must contain a valid IPv4 address", ServiceAnnotationLoadBalancerFloatingIP)
	}

	return regionID, floatingIP, nil
}

// forService returns a load balancer client that uses the Service's region.
func (l *LbaasV2) forService(service *corev1.Service) (*LbaasV2, error) {
	regionID, _, err := resolveServiceRegionAndFloatingIP(service, l.edgecenter.RegionID)
	if err != nil {
		return nil, err
	}
	loadBalancer := l.LoadBalancer
	regionOpts := loadBalancer.opts.LBRegions[strconv.Itoa(regionID)]
	if regionID == l.edgecenter.RegionID && regionOpts == nil {
		return l, nil
	}

	regionConfig := l.edgecenter
	client := l.client
	if regionID != l.edgecenter.RegionID {
		regionConfig.RegionID = regionID
		client, err = NewEdgecenterClient(regionConfig, "edgecenter-cloud-controller-manager", userAgentData...)
		if err != nil {
			return nil, err
		}
		if l.client != nil && l.client.HTTPClient != nil {
			client.HTTPClient.Timeout = l.client.HTTPClient.Timeout
		}
	}

	loadBalancer.client = client
	loadBalancer.edgecenter = regionConfig
	if regionOpts != nil {
		if regionOpts.NetworkID != "" {
			loadBalancer.opts.NetworkID = regionOpts.NetworkID
		}
		if regionOpts.SubnetID != "" {
			loadBalancer.opts.SubnetID = regionOpts.SubnetID
		}
		if regionOpts.FloatingNetworkID != "" {
			loadBalancer.opts.FloatingNetworkID = regionOpts.FloatingNetworkID
		}
	}

	return &LbaasV2{LoadBalancer: loadBalancer}, nil
}

// LBRegion contains network settings for a regional load balancer.
type LBRegion struct {
	NetworkID         string `gcfg:"network-id"`
	SubnetID          string `gcfg:"subnet-id"`
	FloatingNetworkID string `gcfg:"floating-network-id"`
}
