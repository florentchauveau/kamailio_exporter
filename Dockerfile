# build
FROM golang:1.18 as builder

WORKDIR /go/src
COPY . /go/src/
RUN CGO_ENABLED=0 go build -a -o kamailio_exporter

# run
FROM scratch

COPY --from=builder /go/src/kamailio_exporter /kamailio_exporter

LABEL author="Florent CHAUVEAU <florentch@pm.me>"
EXPOSE 9494
ENTRYPOINT [ "/kamailio_exporter" ]
