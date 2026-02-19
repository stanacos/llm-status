VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

.PHONY: build run test vet fmt clean compliance compliance-tracked compliance-history

build:
	go build -ldflags "$(LDFLAGS)" -o ./llm-status .

run:
	go run -ldflags "$(LDFLAGS)" .

test:
	go test ./...

vet:
	go vet ./...

compliance-tracked:
	./scripts/compliance-scan-tracked.sh

compliance-history:
	./scripts/compliance-scan-history.sh

compliance: compliance-tracked compliance-history

fmt:
	gofmt -w *.go

clean:
	rm -f ./llm-status
