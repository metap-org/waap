# mail-mock image. Build context is this directory (standalone module, standard library only):
#   docker build -t waap-mail-mock devtools/mail-mock
FROM golang:1.26 AS build
WORKDIR /src
ENV CGO_ENABLED=0 GOTOOLCHAIN=auto
COPY go.mod ./
COPY . .
RUN go build -trimpath -o /out/mail-mock .

FROM alpine:3
COPY --from=build /out/mail-mock /usr/local/bin/mail-mock
EXPOSE 1025/tcp 1143/tcp 1110/tcp 8025/tcp
ENTRYPOINT ["/usr/local/bin/mail-mock"]
