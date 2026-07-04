.PHONY: build run test vet fmt clean image compose-up compose-down

BINARY:= bin/corestack-gcp
VERSION ?= dev

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$(VERSION) -o $(BINARY) ./cmd/corestack-gcp"

run: build
	CORESTACK_GCP_LISTEN=:4588 $(BINARY)

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

clean:
	rm -rf bin

image:
	docker build -f docker/Dockerfile --build-arg VERSION=$(VERSION) -t corestack-gcp:$(VERSION) .

compose-up:
	docker compose up --build -d

compose-down:
	docker compose down