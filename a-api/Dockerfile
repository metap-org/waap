# analytics-api image. Build context must be the metap-org ROOT: go.mod has
# `replace github.com/metap-org/metap => ../../metap` (pkg/kit only).
#   docker build -f waap/a-api/Dockerfile .
FROM golang:1.26 AS build
WORKDIR /workspace
ENV CGO_ENABLED=0 GOTOOLCHAIN=auto
COPY metap/go.mod metap/go.sum metap/
COPY waap/a-api/go.mod waap/a-api/go.sum waap/a-api/
RUN cd waap/a-api && go mod download
COPY metap/ metap/
COPY waap/a-api/ waap/a-api/
RUN cd waap/a-api && go build -trimpath -o /out/analytics-api ./cmd/analytics-api

FROM alpine:3
COPY --from=build /out/analytics-api /usr/local/bin/analytics-api
EXPOSE 3030
ENTRYPOINT ["/usr/local/bin/analytics-api"]
