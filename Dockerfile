# syntax=docker/dockerfile:1.7
#
# Multi-stage build. The builder compiles the CGO binary against Alpine's
# musl + sqlite headers. Targets:
#   empty  server without docs, bundles are installed at runtime (~25 MiB)
#   full   all offline bundles preinstalled
#   docker build --target full -t marvindocs:full .
#
# The mattn/go-sqlite3 driver is CGO-only and requires the
# sqlite_fts5 build tag, hence CGO_ENABLED=1 in the builder.

ARG GO_VERSION=1.25

FROM golang:${GO_VERSION}-alpine AS builder
RUN apk add --no-cache build-base sqlite-dev
WORKDIR /src

COPY go.mod go.sum ./
COPY parsers/devdocsfmt/go.mod ./parsers/devdocsfmt/
RUN go mod download

COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=1 \
    go build -tags sqlite_fts5 -ldflags="-s -w -X main.version=${VERSION}" \
    -o /out/marvindocs ./cmd/marvindocs

# empty: the server with no docs. Install bundles inside the container and
# restart it:
#   docker exec marvindocs marvindocs bundle install go python
#   docker restart marvindocs
# Keep /var/lib/marvindocs in a named volume: on first start docker copies
# the empty index into it, and installed bundles survive recreates.
FROM alpine:3.20 AS empty
RUN apk add --no-cache ca-certificates sqlite-libs
COPY --from=builder /out/marvindocs /usr/local/bin/marvindocs
# serve and bundle install share the default relative paths
# (data/public/docs, marvindocs.db) under this directory.
WORKDIR /var/lib/marvindocs
RUN mkdir -p data/public/docs && echo '[]' > data/public/docs/docs.json \
    && marvindocs index build
EXPOSE 3333
ENTRYPOINT ["/usr/local/bin/marvindocs"]
CMD ["serve", "--http", "0.0.0.0:3333"]

# full: every bundle of the "bundles" release baked into the image
# (several GB). BUNDLES=all reads the names from SHA256SUMS; pass a list
# such as "go db" to bake only those.
FROM empty AS full
ARG BUNDLES=all
RUN if [ "$BUNDLES" = all ]; then \
      BUNDLES=$(wget -qO- https://github.com/nemoagent/marvindocs/releases/download/bundles/SHA256SUMS \
        | sed -n 's/.*  \(.*\)\.tar\.gz$/\1/p'); \
    fi \
    && marvindocs bundle install $BUNDLES

# Default target: the server for mounted data (docker-compose.yml service
# marvindocs mounts the docs and the index from the host).
FROM empty
