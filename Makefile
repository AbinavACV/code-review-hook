.PHONY: build test lint install tidy clean

BIN := bin/code-review-hook

build:
	go build -o $(BIN) ./cmd/code-review-hook

test:
	go test ./...

lint:
	go vet ./...

install:
	go install ./cmd/code-review-hook

tidy:
	go mod tidy

clean:
	rm -rf bin/ coverage.out coverage.html
