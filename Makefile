.PHONY: test build release fmt vet

test:
	go test ./...

build:
	go build ./...

fmt:
	gofmt -w .

vet:
	go vet ./...

release:
	./scripts/build-release.sh
