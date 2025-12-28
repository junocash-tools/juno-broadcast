.PHONY: build test test-unit test-integration test-e2e fmt tidy clean

BIN_DIR := bin
BIN := $(BIN_DIR)/juno-broadcast

TESTFLAGS ?=

ifneq ($(JUNO_TEST_LOG),)
TESTFLAGS += -v
endif

test-unit:
	go test $(TESTFLAGS) ./...

test-integration:
	go test $(TESTFLAGS) -tags=integration ./...

test-e2e:
	go test $(TESTFLAGS) -tags=e2e ./...

test: test-unit test-integration test-e2e

build:
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN) ./cmd/juno-broadcast

fmt:
	gofmt -w .

tidy:
	go mod tidy

clean:
	rm -rf $(BIN_DIR)
