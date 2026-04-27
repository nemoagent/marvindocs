GO_IMAGE = golang:1.25
DOCKER_RUN = docker run --rm \
	-v "$(PWD)":/src \
	-v "$(PWD)/.gocache":/root/.cache/go-build \
	-v "$(PWD)/.gomodcache":/go/pkg/mod \
	-w /src \
	-e CGO_ENABLED=1 \
	$(GO_IMAGE)

.PHONY: build tidy test vet fmt run-http run-stdio index parsers parsers-test clean

build:
	mkdir -p bin .gocache .gomodcache
	$(DOCKER_RUN) go build -tags sqlite_fts5 -buildvcs=false -o bin/marvindocs ./cmd/marvindocs

tidy:
	mkdir -p .gocache .gomodcache
	$(DOCKER_RUN) go mod tidy

test:
	$(DOCKER_RUN) go test -tags sqlite_fts5 ./...

vet:
	$(DOCKER_RUN) go vet -tags sqlite_fts5 ./...

fmt:
	$(DOCKER_RUN) gofmt -w .

index: build
	./bin/marvindocs index build

run-http: build
	./bin/marvindocs serve --http :3333

run-stdio: build
	./bin/marvindocs serve --stdio

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
			go build -buildvcs=false -o /src/bin/$$name-parser . ; \
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
			go test ./... ; \
	done

clean:
	rm -rf bin .gocache .gomodcache marvindocs.db*
