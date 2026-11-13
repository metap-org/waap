# waf-config-distributor image. Build context must be the metap-org ROOT (the repo checked out as
# `waap/` next to `metap/`): go.mod has `replace github.com/metap-org/metap => ../../metap`.
#   docker build -f waap/control-plane/Dockerfile .
FROM golang:1.26 AS build
WORKDIR /workspace
ENV CGO_ENABLED=0 GOTOOLCHAIN=auto
COPY metap/go.mod metap/go.sum metap/
COPY waap/control-plane/go.mod waap/control-plane/go.sum waap/control-plane/
RUN cd waap/control-plane && go mod download
COPY metap/ metap/
COPY waap/control-plane/ waap/control-plane/
RUN cd waap/control-plane && go build -trimpath -o /out/waf-config-distributor ./cmd/waf-config-distributor

FROM alpine:3
COPY --from=build /out/waf-config-distributor /usr/local/bin/waf-config-distributor
EXPOSE 4100
ENTRYPOINT ["/usr/local/bin/waf-config-distributor"]
