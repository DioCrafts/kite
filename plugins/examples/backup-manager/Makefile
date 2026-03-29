.PHONY: build clean test

BINARY := backup-manager-plugin

build:
	go build -o $(BINARY) .
	cd frontend && pnpm install && pnpm build

clean:
	rm -f $(BINARY)
	rm -rf frontend/dist

test:
	go test -v ./...
