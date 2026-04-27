# syntax=docker/dockerfile:1.7
#
# Multi-stage build. Stage 1 compiles the static-ish CGO binary against
# Alpine's musl + sqlite headers; stage 2 ships only the binary plus
# ca-certificates so the runtime image stays small (~25 MiB).
#
# The mattn/go-sqlite3 driver is CGO-only and requires the
# sqlite_fts5 build tag, hence CGO_ENABLED=1 in the builder.

ARG GO_VERSION=1.25

FROM golang:${GO_VERSION}-alpine AS builder
RUN apk add --no-cache build-base sqlite-dev
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 \
    go build -tags sqlite_fts5 -ldflags="-s -w" \
    -o /out/marvindocs ./cmd/marvindocs

FROM alpine:3.20
RUN apk add --no-cache ca-certificates sqlite-libs
COPY --from=builder /out/marvindocs /usr/local/bin/marvindocs

# Default mount points. Override with bind mounts at runtime:
#   docker run -v /host/docs:/data -v /host/index:/index marvindocs
WORKDIR /app
ENV MARVINDOCS_DOCS_PATH=/data \
    MARVINDOCS_INDEX_PATH=/index/marvindocs.db

EXPOSE 3333
ENTRYPOINT ["/usr/local/bin/marvindocs"]
CMD ["serve", "--http", ":3333", "--docs-path", "/data", "--index-path", "/index/marvindocs.db"]
