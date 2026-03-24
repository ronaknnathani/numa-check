BINARY = numa-check
GOOS   = linux
GOARCH = amd64

.PHONY: build lint test clean

build:
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o $(BINARY) .

lint:
	GOOS=$(GOOS) GOARCH=$(GOARCH) go vet ./...

test:
	GOOS=$(GOOS) GOARCH=$(GOARCH) go test ./...

clean:
	rm -f $(BINARY)
