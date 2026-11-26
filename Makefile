GO ?= go
LINT = golangci-lint

.PHONY: build test lint check clean

build:
	$(GO) build -o . ./cmd/wxreport

test:
	$(GO) test ./...

lint:
	$(LINT) run

check: lint test

clean:
	rm -f wxreport wxreport.exe
