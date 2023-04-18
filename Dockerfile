FROM golang:1.20-alpine

ADD . /go/src/github.com/dewski/wattbox_exporter
RUN go install github.com/dewski/wattbox_exporter@latest

EXPOSE 8181

ENTRYPOINT /go/bin/wattbox_exporter
