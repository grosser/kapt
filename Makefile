# default task since it's first
.PHONY: all
all: build test

BINARY = kapt
$(BINARY): *.go pkg/kapt/*.go go.mod go.sum
	go build -trimpath -o $(BINARY)

.PHONY: build
build: $(BINARY) ## Build binary

.PHONY: test
test: go-testcov ## Unit test with coverage enforcement
	$(GOTESTCOV) ./... -covermode atomic

.PHONY: install
install: ## Install binary
	go install

LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)
GOTESTCOV ?= $(LOCALBIN)/go-testcov
GOTESTCOV_VERSION ?= v1.16.0

.PHONY: go-testcov
go-testcov: $(LOCALBIN) # Download go-testcov (replace existing if incorrect version)
	@(test -f $(GOTESTCOV) && $(GOTESTCOV) version | grep "$(GOTESTCOV_VERSION)" >/dev/null) || \
	(rm -f $(GOTESTCOV) && echo "Installing $(GOTESTCOV) $(GOTESTCOV_VERSION)" && \
	GOBIN=$(LOCALBIN) go install github.com/grosser/go-testcov@$(GOTESTCOV_VERSION))
