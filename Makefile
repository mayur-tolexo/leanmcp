.PHONY: build test vet fmt check
build:
	go build ./...
test:
	go test ./...
vet:
	go vet ./...
fmt:
	gofmt -l .
check: fmt vet build test
