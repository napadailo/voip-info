# voip-info: build, tests, live screens, release archives.
BIN     := bin/voip-info
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build install test vet live run clean snapshot digest

build:
	go build -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/voip-info

install:
	go install -ldflags "$(LDFLAGS)" ./cmd/voip-info

test:
	go test ./...

vet:
	go vet ./...
	gofmt -l . | (! grep .)

# screens over the real feeds, printed as text
live:
	VOIP_INFO_LIVE=1 go test ./internal/ui -run Live -v -count=1

run: build
	./$(BIN)

# the digest of a week to stdout, as cron would run it
digest: build
	./$(BIN) digest -days 7

# release archives into dist/ without publishing (goreleaser)
snapshot:
	goreleaser release --snapshot --clean

clean:
	rm -rf bin dist
