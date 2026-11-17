TAG ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo dev)

.PHONY: build test secret-scan run-local

build:
	go build -ldflags "-X main.version=$(TAG)" -o bin/autopilot ./cmd/autopilot

test:
	go vet ./...
	go test -race -count=1 ./...

secret-scan:
	docker run --rm -v "$(CURDIR)":/repo ghcr.io/gitleaks/gitleaks:v8.30.1 dir -v /repo

# run-local: AUTOPILOT_SETTINGS is the settings folder (<workspace folder>/.workspace/autopilot). serve reads the
# workspace registry from <workspace folder>/.workspace/ and takes the agterm socket from AGTERM_SOCKET.
run-local:
	@test -n "$(AUTOPILOT_SETTINGS)" || { echo "run-local: set AUTOPILOT_SETTINGS=<workspace folder>/.workspace/autopilot" >&2; exit 2; }
	go run ./cmd/autopilot serve --settings "$(AUTOPILOT_SETTINGS)"
