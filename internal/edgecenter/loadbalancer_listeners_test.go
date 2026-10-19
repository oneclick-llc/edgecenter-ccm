package edgecenter

import (
	"fmt"
	"net/http"
	"testing"

	edgecloud "github.com/Edge-Center/edgecentercloud-go/v2"
)

func TestBuildListenerUpdateRequestSkipsUnsetTimeouts(t *testing.T) {
	listener := &edgecloud.Listener{
		TimeoutClientData:    edgecloud.PtrTo(50000),
		TimeoutMemberData:    edgecloud.PtrTo(50000),
		TimeoutMemberConnect: edgecloud.PtrTo(5000),
	}
	opts := &ServiceOptions{Timeouts: &ListenerTimeoutConfig{}}

	if _, needsUpdate := buildListenerUpdateRequest(listener, opts); needsUpdate {
		t.Fatal("listener without timeout annotations must not be updated")
	}
}

func TestBuildListenerUpdateRequestAppliesChangedTimeouts(t *testing.T) {
	listener := &edgecloud.Listener{
		TimeoutClientData:    edgecloud.PtrTo(50000),
		TimeoutMemberData:    edgecloud.PtrTo(50000),
		TimeoutMemberConnect: edgecloud.PtrTo(5000),
	}
	opts := &ServiceOptions{Timeouts: &ListenerTimeoutConfig{
		TimeoutClientData: edgecloud.PtrTo(3600000),
		TimeoutMemberData: edgecloud.PtrTo(50000),
	}}

	req, needsUpdate := buildListenerUpdateRequest(listener, opts)
	if !needsUpdate {
		t.Fatal("changed client data timeout must update the listener")
	}
	if req.TimeoutClientData == nil || *req.TimeoutClientData != 3600000 {
		t.Fatalf("TimeoutClientData = %v, want 3600000", req.TimeoutClientData)
	}
	if req.TimeoutMemberData != nil || req.TimeoutMemberConnect != nil {
		t.Fatalf("unchanged timeouts must stay unset, got member data %v, member connect %v", req.TimeoutMemberData, req.TimeoutMemberConnect)
	}
}

func TestIsHTTPNotFound(t *testing.T) {
	notFound := &edgecloud.ResponseError{Response: &http.Response{StatusCode: http.StatusNotFound, Request: &http.Request{}}}
	conflict := &edgecloud.ResponseError{Response: &http.Response{StatusCode: http.StatusConflict, Request: &http.Request{}}}

	if !isHTTPNotFound(fmt.Errorf("failed to get instance: %w", notFound)) {
		t.Fatal("wrapped 404 must be detected")
	}
	if isHTTPNotFound(conflict) {
		t.Fatal("409 must not be treated as not found")
	}
	if isHTTPNotFound(nil) {
		t.Fatal("nil error must not be treated as not found")
	}
}
