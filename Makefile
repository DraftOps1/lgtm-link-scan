GO ?= go
DEMO_ENV_FILE ?= examples/docker-compose/.env.bad
COMPOSE = docker compose -f examples/docker-compose/compose.yaml --env-file $(DEMO_ENV_FILE)

.PHONY: fmt test build doctor demo-up-bad demo-up-good demo-down demo-ps demo-logs

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './vendor/*')

test:
	$(GO) test ./...

build:
	$(GO) build ./cmd/lgtm-link-scan

doctor:
	$(GO) run ./cmd/lgtm-link-scan doctor -c lgtm-link-scan.yaml

demo-up-bad:
	$(COMPOSE) up --build -d

demo-up-good:
	$(MAKE) DEMO_ENV_FILE=examples/docker-compose/.env.good demo-up-bad

demo-down:
	$(COMPOSE) down -v

demo-ps:
	$(COMPOSE) ps

demo-logs:
	$(COMPOSE) logs -f demoapp alloy lgtm
