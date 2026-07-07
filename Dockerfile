# build
FROM golang:1.25 AS builder

WORKDIR /go/src
COPY go.mod go.sum /go/src/
RUN go mod download
COPY . /go/src/
RUN CGO_ENABLED=0 go build -o kamailio_exporter

# run
FROM scratch

COPY --from=builder /go/src/kamailio_exporter /kamailio_exporter

LABEL author="Florent CHAUVEAU <florentch@pm.me>"
EXPOSE 9494
ENTRYPOINT [ "/kamailio_exporter" ]
