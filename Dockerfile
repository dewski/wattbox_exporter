FROM golang:1.14.2-alpine

ADD . /go/src/github.com/dewski/wattbox_exporter
RUN go install github.com/dewski/wattbox_exporter

EXPOSE 8181

ENTRYPOINT /go/bin/wattbox_exporter
