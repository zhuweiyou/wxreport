GO ?= go

.PHONY: test clean

test:
	$(GO) test ./...

clean:
	rm -rf bin
