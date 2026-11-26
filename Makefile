GO ?= go

.PHONY: build test clean

build:
	$(GO) build -o . ./cmd/wxreport

test:
	$(GO) test ./...

clean:
	rm -f wxreport wxreport.exe
