GO_IMAGE = golang:1.25
DOCKER_RUN = docker run --rm \
	-v "$(PWD)":/src \
	-v "$(PWD)/.gocache":/root/.cache/go-build \
	-v "$(PWD)/.gomodcache":/go/pkg/mod \
	-w /src \
	-e CGO_ENABLED=1 \
	$(GO_IMAGE)

# Vector search (optional): see docs/en/vectors.md.
ONNX_DIR = data/onnx

.PHONY: build build-onnx onnx-fetch tidy test test-full vet lint vuln check fmt run-http run-stdio run-onnx index embed update parsers parsers-test clean clean-index

build:
	mkdir -p bin .gocache .gomodcache
	$(DOCKER_RUN) go build -tags sqlite_fts5 -buildvcs=false -o bin/marvindocs ./cmd/marvindocs

# Binary with the ONNX embedder; needs $(ONNX_DIR)/lib from onnx-fetch.
build-onnx: onnx-fetch
	mkdir -p bin .gocache .gomodcache
	$(DOCKER_RUN) env CGO_LDFLAGS="-L/src/$(ONNX_DIR)/lib" \
		go build -tags "sqlite_fts5 onnx" -buildvcs=false -o bin/marvindocs ./cmd/marvindocs

# Download libtokenizers.a, libonnxruntime.so and the bge-m3 model into $(ONNX_DIR).
onnx-fetch:
	ONNX_DIR=$(ONNX_DIR) ./scripts/fetch-onnx.sh

tidy:
	mkdir -p .gocache .gomodcache
	$(DOCKER_RUN) go mod tidy

test:
	$(DOCKER_RUN) go test -short -tags sqlite_fts5 ./...

test-full:
	$(DOCKER_RUN) go test -timeout 30m -tags sqlite_fts5 ./...

vet:
	$(DOCKER_RUN) go vet -tags sqlite_fts5 ./...

lint:
	mkdir -p .gocache .gomodcache
	docker run --rm -v "$(PWD)":/src -v "$(PWD)/.gocache":/root/.cache/go-build \
		-v "$(PWD)/.gomodcache":/go/pkg/mod -w /src -e CGO_ENABLED=1 \
		golangci/golangci-lint:v2.13.2 golangci-lint run ./...

vuln:
	mkdir -p .gocache .gomodcache
	$(DOCKER_RUN) go run golang.org/x/vuln/cmd/govulncheck@v1.1.4 -tags sqlite_fts5 ./...

check: vet lint vuln test

fmt:
	$(DOCKER_RUN) gofmt -w cmd internal parsers

index: build
	./bin/marvindocs index build

run-http: build
	./bin/marvindocs serve --http :3333

run-stdio: build
	./bin/marvindocs serve --stdio

run-onnx: build-onnx
	./bin/marvindocs serve --http :3333 --embedder onnx \
		--model-dir $(ONNX_DIR)/model --onnxruntime $(ONNX_DIR)/lib/libonnxruntime.so

# Embed chunks that have no vector yet (incremental). Embedder comes from
# .env (MARVINDOCS_EMBEDDER, default onnx = local CPU, slow).
embed:
	./scripts/update.sh --embed-only

# Safe end-to-end refresh: devdocs sync, parsers, incremental index, embed.
update:
	./scripts/update.sh

# Build every parser binary into bin/<name>-parser. Each parser is its
# own Go module so we iterate; no CGO needed for parsers, hence the
# bare go image.
parsers:
	mkdir -p bin .gocache .gomodcache
	@for d in parsers/*/; do \
		name=$$(basename $$d); \
		if [ "$$name" = "devdocsfmt" ]; then continue; fi; \
		echo "==> building $$name"; \
		docker run --rm \
			-v "$(PWD)":/src \
			-v "$(PWD)/.gocache":/root/.cache/go-build \
			-v "$(PWD)/.gomodcache":/go/pkg/mod \
			-w /src/$$d \
			$(GO_IMAGE) \
			go build -buildvcs=false -o /src/bin/$$name-parser . || exit 1; \
	done

parsers-test:
	@for d in parsers/*/; do \
		echo "==> testing $$(basename $$d)"; \
		docker run --rm \
			-v "$(PWD)":/src \
			-v "$(PWD)/.gocache":/root/.cache/go-build \
			-v "$(PWD)/.gomodcache":/go/pkg/mod \
			-w /src/$$d \
			$(GO_IMAGE) \
			go test ./... || exit 1; \
	done

# Build artefacts only; the index is kept (hours of embedding live there).
clean:
	rm -rf bin .gocache .gomodcache

# Delete the SQLite index including all chunk embeddings.
clean-index:
	rm -f marvindocs.db marvindocs.db-*
