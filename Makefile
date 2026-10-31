.PHONY: build install docker test vet fmt lint sync sync-baseline smoke clean

BIN_DIR := bin
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

build:
	@mkdir -p $(BIN_DIR)
	go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/newapi-operator ./cmd/newapi-operator
	go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/routesync ./cmd/routesync
	@echo "built $(BIN_DIR)/newapi-operator $(BIN_DIR)/routesync ($(VERSION))"

install:
	go install ./cmd/newapi-operator

docker:
	docker build --build-arg VERSION=$(VERSION) -t newapi-operator:$(VERSION) .

test:
	go test ./... -race -count=1

vet:
	go vet ./...

fmt:
	gofmt -w .
	@test -z "$$(gofmt -l .)" || (echo "unformatted files:"; gofmt -l .; exit 1)

lint: vet fmt

# Regenerate the route table from the newest New API release and show the diff.
sync:
	go run ./cmd/routesync --tag latest --report
	@git --no-pager diff --stat -- internal/spec || true

# Refresh the committed baseline after a deliberate, reviewed sync.
sync-baseline:
	go run ./cmd/routesync --tag latest --out /dev/null --write-baseline
	@echo "baseline refreshed — review the diff before committing"

# Fail if the table no longer matches the baseline (same check CI runs).
drift:
	go run ./cmd/routesync --tag latest --out /dev/null --check

# End-to-end check against a live instance. Needs NEWAPI_BASE_URL.
smoke: build
	python3 scripts/smoke_test.py

clean:
	rm -rf $(BIN_DIR)