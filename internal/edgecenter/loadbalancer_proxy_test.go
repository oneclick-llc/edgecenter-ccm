package edgecenter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	edgecloud "github.com/Edge-Center/edgecentercloud-go/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestProxyAnnotationSetsPoolProtocol(t *testing.T) {
	controller := &LbaasV2{LoadBalancer: LoadBalancer{opts: LoadBalancerOpts{
		UseOctavia: true, NetworkID: "network", FloatingNetworkID: "floating",
	}}}
	for _, enabled := range []bool{false, true} {
		annotation := "false"
		want := edgecloud.LBPoolProtocolTCP
		if enabled {
			annotation = "true"
			want = edgecloud.LBPoolProtocolProxy
		}
		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{ServiceAnnotationLoadBalancerProxyEnabled: annotation}},
			Spec:       corev1.ServiceSpec{SessionAffinity: corev1.ServiceAffinityNone, Ports: []corev1.ServicePort{{Name: "https", Port: 443, Protocol: corev1.ProtocolTCP}}},
		}
		opts, err := controller.resolveServiceOptions(context.Background(), svc, nil)
		if err != nil {
			t.Fatal(err)
		}
		listener := buildListenerCreateRequest("lb", "example", svc.Spec.Ports[0], opts)
		if listener.Protocol != edgecloud.ListenerProtocolTCP {
			t.Fatalf("listener protocol = %s, want TCP", listener.Protocol)
		}
		pool := controller.buildPoolCreateRequest(&edgecloud.Listener{Protocol: listener.Protocol}, "example", svc.Spec.Ports[0], svc, opts)
		if pool.Protocol != want {
			t.Fatalf("proxy %t: pool protocol = %s, want %s", enabled, pool.Protocol, want)
		}
	}
}

func TestProxyAnnotationRejectsIncompatibleListeners(t *testing.T) {
	controller := &LbaasV2{LoadBalancer: LoadBalancer{opts: LoadBalancerOpts{
		UseOctavia: true, NetworkID: "network", FloatingNetworkID: "floating",
	}}}
	for _, test := range []struct {
		name, extraAnnotation string
		protocol              corev1.Protocol
	}{
		{name: "UDP", protocol: corev1.ProtocolUDP},
		{name: "HTTP headers", protocol: corev1.ProtocolTCP, extraAnnotation: ServiceAnnotationLoadBalancerXForwardedFor},
		{name: "TLS termination", protocol: corev1.ProtocolTCP, extraAnnotation: ServiceAnnotationLoadBalancerDefaultTLSContainerRef},
	} {
		t.Run(test.name, func(t *testing.T) {
			annotations := map[string]string{ServiceAnnotationLoadBalancerProxyEnabled: "true"}
			if test.extraAnnotation != "" {
				annotations[test.extraAnnotation] = "true"
			}
			svc := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Annotations: annotations},
				Spec:       corev1.ServiceSpec{SessionAffinity: corev1.ServiceAffinityNone, Ports: []corev1.ServicePort{{Port: 443, Protocol: test.protocol}}},
			}
			if _, err := controller.resolveServiceOptions(context.Background(), svc, nil); err == nil {
				t.Fatal("incompatible PROXY configuration was accepted")
			}
		})
	}
}

func TestExistingPoolsAreNotMutatedOnProtocolChange(t *testing.T) {
	const lbID = "11111111-1111-4111-8111-111111111111"
	const listenerID = "22222222-2222-4222-8222-222222222222"
	const poolID = "33333333-3333-4333-8333-333333333333"
	for _, test := range []struct {
		name      string
		protocol  edgecloud.LoadbalancerPoolProtocol
		proxy     bool
		wantError bool
	}{
		{"unchanged TCP", edgecloud.LBPoolProtocolTCP, false, false},
		{"TCP to PROXY", edgecloud.LBPoolProtocolTCP, true, true},
		{"unchanged PROXY", edgecloud.LBPoolProtocolProxy, true, false},
		{"PROXY to TCP", edgecloud.LBPoolProtocolProxy, false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.Method != http.MethodGet || !strings.HasSuffix(r.URL.Path, "/lbpools/1/10") {
					t.Errorf("unexpected API request: %s %s", r.Method, r.URL.Path)
					http.Error(w, "unexpected request", http.StatusBadRequest)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"count": 1, "results": []any{map[string]any{
					"id": poolID, "protocol": test.protocol,
					"listeners": []any{map[string]string{"id": listenerID}},
				}}})
			}))
			defer server.Close()
			client, err := edgecloud.New(server.Client(), edgecloud.SetBaseURL(server.URL+"/"), edgecloud.SetProject(1), edgecloud.SetRegion(10))
			if err != nil {
				t.Fatal(err)
			}
			controller := &LbaasV2{LoadBalancer: LoadBalancer{client: client}}
			port := corev1.ServicePort{Name: "https", Port: 443, Protocol: corev1.ProtocolTCP}
			svc := &corev1.Service{Spec: corev1.ServiceSpec{SessionAffinity: corev1.ServiceAffinityNone, Ports: []corev1.ServicePort{port}}}
			listeners := map[string]*edgecloud.Listener{servicePortKey(port): {ID: listenerID, Protocol: edgecloud.ListenerProtocolTCP}}
			pools, obsolete, err := controller.ensureLoadBalancerPools(context.Background(), &edgecloud.Loadbalancer{ID: lbID}, svc, listeners, &ServiceOptions{UseProxyProtocol: test.proxy})
			if (err != nil) != test.wantError {
				t.Fatalf("error = %v, wantError %t", err, test.wantError)
			}
			if test.wantError && !strings.Contains(err.Error(), "maintenance window") {
				t.Fatalf("unexpected error: %v", err)
			}
			if !test.wantError && (len(pools) != 1 || len(obsolete) != 0 || pools[servicePortKey(port)].ID != poolID) {
				t.Fatalf("existing pool was not retained: pools=%v obsolete=%v", pools, obsolete)
			}
			if requests != 1 {
				t.Fatalf("API requests = %d, want one read", requests)
			}
		})
	}
}
