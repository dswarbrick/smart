.PHONY:
	test fmt check

all: check test

test:
	@go test -v ./...

fmt:
	@go fmt ./...

check: fmt
	@go vet ./...
