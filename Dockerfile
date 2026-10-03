# Build on the host's own architecture and cross-compile: the Go toolchain crashes
# under QEMU emulation when building the other architecture of a multi-arch image.
FROM --platform=$BUILDPLATFORM golang:1.20-alpine AS build
ARG TARGETOS TARGETARCH

WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /wattbox_exporter .

FROM golang:1.20-alpine

COPY --from=build /wattbox_exporter /go/bin/wattbox_exporter

EXPOSE 8181

ENTRYPOINT /go/bin/wattbox_exporter
