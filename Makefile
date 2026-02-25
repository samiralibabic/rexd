SHELL := /usr/bin/env bash

.PHONY: build test verify verify-stdio verify-http verify-ws tidy clean

build:
	go build -o rexd ./cmd/rexd

test:
	go test ./...

verify: build verify-stdio verify-http
	@echo "Verification complete."

verify-stdio: build
	bash ./scripts/verify-stdio.sh

verify-http: build
	bash ./scripts/verify-http.sh

verify-ws: build
	bash ./scripts/verify-ws.sh

tidy:
	go mod tidy

clean:
	rm -f ./rexd ./rexd_verify.txt
