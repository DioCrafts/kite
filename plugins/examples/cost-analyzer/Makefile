.PHONY: build clean test

PLUGIN_NAME = cost-analyzer
BINARY = $(PLUGIN_NAME)

build:
	go build -o $(BINARY) .
	cd frontend && pnpm build

clean:
	rm -f $(BINARY)
	rm -rf frontend/dist

test:
	go test ./...
