# dns-mock image. Build context is this directory (standalone module, no metap dependency):
#   docker build -t waap-dns-mock devtools/dns-mock
FROM golang:1.26 AS build
WORKDIR /src
ENV CGO_ENABLED=0 GOTOOLCHAIN=auto
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -trimpath -o /out/dns-mock .

FROM alpine:3
COPY --from=build /out/dns-mock /usr/local/bin/dns-mock
COPY seed.dev.json /etc/dns-mock/seed.dev.json
EXPOSE 5353/udp 5353/tcp 8053/tcp
ENTRYPOINT ["/usr/local/bin/dns-mock"]
