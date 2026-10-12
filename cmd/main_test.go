package main

import (
	"slices"
	"testing"

	"k8s.io/cloud-provider/app"
	"k8s.io/cloud-provider/names"
	"k8s.io/cloud-provider/options"
)

func TestServiceControllerAliasIsCanonicalized(t *testing.T) {
	controllerOptions, err := options.NewCloudControllerManagerOptions()
	if err != nil {
		t.Fatalf("create controller options: %v", err)
	}
	controllerOptions.Generic.Controllers = []string{"service"}

	controllerNames, aliases := controllerConfiguration()
	config, err := controllerOptions.Config(
		controllerNames,
		app.ControllersDisabledByDefault.List(),
		aliases,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("configure controller manager: %v", err)
	}

	if !slices.Contains(config.ComponentConfig.Generic.Controllers, names.ServiceLBController) {
		t.Fatalf("service alias was not canonicalized: %v", config.ComponentConfig.Generic.Controllers)
	}
}
