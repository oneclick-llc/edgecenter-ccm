FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
WORKDIR /build
ADD go.mod go.sum /build/
RUN go mod download -x
ADD cmd /build/cmd
ADD internal /build/internal
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags "-X ec-ccm/internal/version.Version=${VERSION}" -o ./ec-ccm ./cmd/main.go

FROM alpine:3.24
COPY --from=builder /build/ec-ccm /usr/local/bin/
ENTRYPOINT ["ec-ccm"]
