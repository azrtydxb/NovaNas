.PHONY: build test vet lint clean

BIN := bin/novanas-installer

build:
	GOWORK=off CGO_ENABLED=0 go build -ldflags="-s -w" -o $(BIN) .

test:
	GOWORK=off go test ./...

vet:
	GOWORK=off go vet ./...

lint:
	golangci-lint run

clean:
	rm -rf bin
