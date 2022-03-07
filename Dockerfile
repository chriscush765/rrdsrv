FROM golang:1.16 as BUILDER
WORKDIR /usr/src/app
COPY RRDSRV ./
RUN CGO_ENABLED=0 go build -a -installsuffix cgo

FROM alpine:latest
WORKDIR /srv
COPY --from=BUILDER /usr/src/app/rrdsrv ./

# @TODO use a volume instead of copying the over lol
COPY rrd /srv/rrd
ENTRYPOINT ["./rrdsrv"]