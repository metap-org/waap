# One image recipe for every Go binary in this module — pick it with --build-arg SERVICE=<cmd dir>
# (zones-service, scanning-service, alerting-service, waf-graphql-gateway).
#
# Build context must be the metap-org ROOT (docker-compose.yml uses `context: ../../`): go.mod
# has `replace github.com/metap-org/metap => ../../metap`, so ../metap has to be inside the
# context. The root .dockerignore keeps node_modules/target/.git out of it.
FROM golang:1.26 AS build
WORKDIR /workspace
ENV CGO_ENABLED=0 GOTOOLCHAIN=auto
COPY metap/go.mod metap/go.sum metap/
COPY waap/data-plane/go.mod waap/data-plane/go.sum waap/data-plane/
RUN cd waap/data-plane && go mod download
COPY metap/ metap/
COPY waap/data-plane/ waap/data-plane/
ARG SERVICE
RUN test -n "$SERVICE" && cd waap/data-plane && go build -trimpath -o /out/app ./cmd/${SERVICE}

FROM alpine:3
COPY --from=build /out/app /usr/local/bin/app
ENTRYPOINT ["/usr/local/bin/app"]
