
    BINARY=zephyrcache
    PKG=./...

    .PHONY: build run test fmt lint proto

    build:
	go build -o bin/$(BINARY) ./cmd/server

    run: build
	./bin/$(BINARY)

    bench:
	go run ./cmd/bench

    test:
	go test $(PKG) -v

    fmt:
	gofmt -w .

    proto:
	@echo "Proto generation not wired yet. Install buf/protoc & generate stubs into ./proto."
