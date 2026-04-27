# Go documentation via Docker

If you do not want multiple local Go toolchains, run `go doc` inside a container.
This gives you docs from the selected Go image version without installing Go on the host.

## Quick start

```bash
# Package documentation
./scripts/go-doc.sh fmt

# Symbol documentation
./scripts/go-doc.sh io.Reader

# All exported package items
./scripts/go-doc.sh -all net/http
```

Default image: `golang:1.25`.

## Select Go version

```bash
GO_IMAGE=golang:1.24 ./scripts/go-doc.sh slices
GO_IMAGE=golang:1.23 ./scripts/go-doc.sh maps
```

## How it works

- `docker-compose.yml` includes a `godoc` service.
- The service uses `go doc` as its entrypoint.
- `scripts/go-doc.sh` forwards arguments to `docker compose run --rm godoc ...`.

## Notes

- No host Go installation is required (Docker only).
- On first use of a specific `GO_IMAGE`, Docker pulls the image.
- Output matches the Go version inside the selected image.
