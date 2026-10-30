TOOLS_DIR ?= $(CURDIR)/.tools

.PHONY: build test check coverage fuzz tools lint scan cross docs
build:
	go build -o bin/tracefull ./cmd/tracefull

test:
	go test ./...

check: test
	test -z "$$(gofmt -l cmd internal)"
	go vet ./...
	go test -race ./...
	$(MAKE) coverage lint docs build

coverage:
	python3 scripts/coverage.py

fuzz:
	go test ./internal/importer -run '^$$' -fuzz FuzzParse -fuzztime 30s -parallel 4
	go test ./internal/driver/cumulus -run '^$$' -fuzz FuzzParse -fuzztime 30s -parallel 4
	go test ./internal/driver/junos -run '^$$' -fuzz FuzzParse -fuzztime 30s -parallel 4
	go test ./internal/driver/eos -run '^$$' -fuzz FuzzParse -fuzztime 30s -parallel 4
	go test ./internal/driver/linuxfrr -run '^$$' -fuzz FuzzParse -fuzztime 30s -parallel 4
	go test ./internal/driver/iosxr -run '^$$' -fuzz FuzzParse -fuzztime 30s -parallel 4

tools:
	CGO_ENABLED=0 GOBIN=$(TOOLS_DIR) go install honnef.co/go/tools/cmd/staticcheck@v0.8.2
	CGO_ENABLED=0 GOBIN=$(TOOLS_DIR) go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0
	CGO_ENABLED=0 GOBIN=$(TOOLS_DIR) go install golang.org/x/vuln/cmd/govulncheck@v1.9.0

lint:
	$(TOOLS_DIR)/staticcheck ./...
	$(TOOLS_DIR)/golangci-lint run ./...

scan:
	$(TOOLS_DIR)/govulncheck ./...

cross:
	python3 scripts/cross_build.py

docs:
	python3 scripts/docs_check.py
