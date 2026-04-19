GO_IMAGE = golang:1.25
DOCKER_RUN = docker run --rm \
	-v "$(PWD)":/src \
	-v "$(PWD)/.gocache":/root/.cache/go-build \
	-v "$(PWD)/.gomodcache":/go/pkg/mod \
	-w /src \
	-e CGO_ENABLED=0 \
	$(GO_IMAGE)

.PHONY: build tidy test vet fmt run-http run-stdio index clean

build:
	mkdir -p bin .gocache .gomodcache
	$(DOCKER_RUN) go build -buildvcs=false -o bin/marvindocs ./cmd/marvindocs

tidy:
	mkdir -p .gocache .gomodcache
	$(DOCKER_RUN) go mod tidy

test:
	$(DOCKER_RUN) go test ./...

vet:
	$(DOCKER_RUN) go vet ./...

fmt:
	$(DOCKER_RUN) gofmt -w .

index: build
	./bin/marvindocs index build

run-http: build
	./bin/marvindocs serve --http :3333

run-stdio: build
	./bin/marvindocs serve --stdio

clean:
	rm -rf bin .gocache .gomodcache marvindocs.db*
