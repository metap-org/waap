# One image recipe for every Go binary of the root module (the m-* and c-* services) — pick it with
# --build-arg SERVICE=<package dir>, e.g. m-management/cmd/management-service or
# c-config-distributor/cmd/waf-config-distributor.
#
# Build context must be the metap-org ROOT (docker-compose.yml uses `context: ../../`): go.mod
# has `replace github.com/metap-org/metap => ../metap`, so ../metap has to be inside the
# context. The root .dockerignore keeps node_modules/target/.git out of it.
FROM golang:1.26 AS build
WORKDIR /workspace
ENV CGO_ENABLED=0 GOTOOLCHAIN=auto
COPY metap/go.mod metap/go.sum metap/
COPY waap/go.mod waap/go.sum waap/
RUN cd waap && go mod download
COPY metap/ metap/
COPY waap/ waap/
ARG SERVICE
RUN test -n "$SERVICE" && cd waap && go build -trimpath -o /out/app ./${SERVICE}

FROM alpine:3
COPY --from=build /out/app /usr/local/bin/app
ENTRYPOINT ["/usr/local/bin/app"]
