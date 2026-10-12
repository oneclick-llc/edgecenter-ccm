VERSION=v2.2.6
ARCH=amd64
GOOS=linux
PLATFORM=linux/amd64
REGISTRY=ghcr.io/oneclick-llc/edgecenter-ccm
BINARY=bin/ec-ccm

build:
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(ARCH) go build -ldflags "-X ec-ccm/internal/version.Version=$(VERSION)" -o $(BINARY) cmd/main.go

image-build: build
	docker build --platform $(PLATFORM) --build-arg VERSION=$(VERSION) -t $(REGISTRY):$(VERSION) .

image-push: image-build
	docker push $(REGISTRY):$(VERSION)
