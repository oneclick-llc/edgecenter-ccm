package edgecenter

import (
	"context"
	"net"
	"strings"
	"testing"

	edgecloud "github.com/Edge-Center/edgecentercloud-go/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestResolveServiceRegionAndFloatingIP(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		defaultID   int
		wantRegion  int
		wantIP      string
		wantError   bool
	}{
		{
			name:       "use configured region without static address",
			defaultID:  10,
			wantRegion: 10,
		},
		{
			name: "region requires address",
			annotations: map[string]string{
				ServiceAnnotationLoadBalancerRegionID: "20",
			},
			wantError: true,
		},
		{
			name: "address requires region",
			annotations: map[string]string{
				ServiceAnnotationLoadBalancerFloatingIP: "192.0.2.20",
			},
			wantError: true,
		},
		{
			name: "parse regional static address",
			annotations: map[string]string{
				ServiceAnnotationLoadBalancerRegionID:   " 20 ",
				ServiceAnnotationLoadBalancerFloatingIP: " 192.0.2.20 ",
			},
			wantRegion: 20,
			wantIP:     "192.0.2.20",
		},
		{
			name: "reject nonnumeric region",
			annotations: map[string]string{
				ServiceAnnotationLoadBalancerRegionID:   "moscow",
				ServiceAnnotationLoadBalancerFloatingIP: "192.0.2.20",
			},
			wantError: true,
		},
		{
			name: "reject IPv6 address",
			annotations: map[string]string{
				ServiceAnnotationLoadBalancerRegionID:   "20",
				ServiceAnnotationLoadBalancerFloatingIP: "2001:db8::20",
			},
			wantError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Annotations: test.annotations}}
			regionID, floatingIP, err := resolveServiceRegionAndFloatingIP(service, test.defaultID)
			if (err != nil) != test.wantError {
				t.Fatalf("resolveServiceRegionAndFloatingIP() error = %v, wantError %v", err, test.wantError)
			}
			if err != nil {
				return
			}
			if regionID != test.wantRegion {
				t.Errorf("region ID = %d, want %d", regionID, test.wantRegion)
			}
			gotIP := ""
			if floatingIP != nil {
				gotIP = floatingIP.String()
			}
			if gotIP != test.wantIP {
				t.Errorf("floating IP = %q, want %q", gotIP, test.wantIP)
			}
		})
	}
}

func TestReadConfigLoadsRegionalLoadBalancerOptions(t *testing.T) {
	config, err := ReadConfig(strings.NewReader(`[LoadBalancerRegion "20"]
network-id = "network-20"
subnet-id = "subnet-20"
floating-network-id = "floating-network-20"
`))
	if err != nil {
		t.Fatalf("ReadConfig() error = %v", err)
	}

	region := config.LoadBalancer.LBRegions["20"]
	if region == nil {
		t.Fatal("ReadConfig() did not load region 20")
	}
	if region.NetworkID != "network-20" || region.SubnetID != "subnet-20" || region.FloatingNetworkID != "floating-network-20" {
		t.Fatalf("regional settings = %+v", region)
	}
}

func TestReadConfigRejectsNonNumericRegionalSection(t *testing.T) {
	_, err := ReadConfig(strings.NewReader(`[LoadBalancerRegion "moscow"]
subnet-id = "subnet-moscow"
`))
	if err == nil {
		t.Fatal("ReadConfig() accepted a nonnumeric region ID")
	}
}

func TestForServiceUsesRegionalClientAndNetworkSettings(t *testing.T) {
	controller := &LbaasV2{LoadBalancer: LoadBalancer{
		client: edgecloud.NewClient(nil),
		opts: LoadBalancerOpts{
			NetworkID: "default-network",
			LBRegions: map[string]*LBRegion{
				"20": {
					NetworkID:         "network-20",
					SubnetID:          "subnet-20",
					FloatingNetworkID: "floating-network-20",
				},
			},
		},
		edgecenter: EdgecenterOpts{
			ApiURL:    "https://api.example.test",
			ApiToken:  "test-token",
			ProjectID: 1,
			RegionID:  10,
		},
	}}
	service := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
		ServiceAnnotationLoadBalancerRegionID:   "20",
		ServiceAnnotationLoadBalancerFloatingIP: "192.0.2.20",
	}}}

	regional, err := controller.forService(service)
	if err != nil {
		t.Fatalf("forService() error = %v", err)
	}
	if regional == controller {
		t.Fatal("forService() returned the default regional controller")
	}
	if regional.edgecenter.RegionID != 20 {
		t.Errorf("regional ID = %d, want 20", regional.edgecenter.RegionID)
	}
	if regional.opts.NetworkID != "network-20" || regional.opts.SubnetID != "subnet-20" || regional.opts.FloatingNetworkID != "floating-network-20" {
		t.Errorf("regional network settings = %+v", regional.opts)
	}
	if controller.edgecenter.RegionID != 10 || controller.opts.NetworkID != "default-network" {
		t.Fatal("forService() changed the default controller settings")
	}
}

func TestValidateStaticFloatingIPAssociation(t *testing.T) {
	address := net.ParseIP("192.0.2.20")
	tests := []struct {
		name         string
		floatingIPs  []edgecloud.FloatingIP
		expectedPort string
		wantError    bool
	}{
		{
			name:        "free address",
			floatingIPs: []edgecloud.FloatingIP{{FloatingIPAddress: address.String()}},
		},
		{
			name:         "address already assigned to this service",
			floatingIPs:  []edgecloud.FloatingIP{{FloatingIPAddress: address.String(), PortID: "vip-port"}},
			expectedPort: "vip-port",
		},
		{
			name:        "address assigned to another service",
			floatingIPs: []edgecloud.FloatingIP{{FloatingIPAddress: address.String(), PortID: "other-port"}},
			wantError:   true,
		},
		{
			name:        "address missing from region",
			floatingIPs: []edgecloud.FloatingIP{{FloatingIPAddress: "192.0.2.21"}},
			wantError:   true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateStaticFloatingIPAssociation(test.floatingIPs, address, test.expectedPort, 20)
			if (err != nil) != test.wantError {
				t.Fatalf("validateStaticFloatingIPAssociation() error = %v, wantError %v", err, test.wantError)
			}
		})
	}
}

func TestEnsureLoadBalancerRejectsStaticAddressChange(t *testing.T) {
	controller := &LbaasV2{LoadBalancer: LoadBalancer{edgecenter: EdgecenterOpts{RegionID: 20}}}
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
			ServiceAnnotationLoadBalancerRegionID:   "20",
			ServiceAnnotationLoadBalancerFloatingIP: "192.0.2.20",
		}},
		Status: corev1.ServiceStatus{LoadBalancer: corev1.LoadBalancerStatus{
			Ingress: []corev1.LoadBalancerIngress{{IP: "192.0.2.21"}},
		}},
	}
	_, err := controller.ensureLoadBalancer(context.Background(), "test", service, nil)
	if err == nil || err.Error() != "static floating IP cannot change on an existing Service; delete and recreate the Service" {
		t.Fatalf("ensureLoadBalancer() error = %v", err)
	}
}
