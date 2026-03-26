BINARY = numa-check
GOOS   = linux
GOARCH = amd64

.PHONY: build lint test test-cover clean

build:
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o $(BINARY) .

lint:
	GOOS=$(GOOS) GOARCH=$(GOARCH) go vet ./...

test:
	go test ./...

test-cover:
	go test -cover ./...

clean:
	rm -f $(BINARY)
