.PHONY: web-deps web web-check web-dev lint fmt mocks unit-test container-test e2e-test test build

# internal/pihole and e2e start real Pi-hole containers, so they need a docker
# socket, and they starve each other if they run at the same time: -p 1.
CONTAINER_PKGS := ./internal/pihole/ ./e2e/

web-deps:
	npm --prefix web ci

web:
	npm --prefix web run build

web-check:
	npm --prefix web run check

web-dev:
	npm --prefix web run dev

lint:
	golangci-lint run

fmt:
	golangci-lint fmt

mocks:
	mockery --config .mockery.yaml

unit-test:
	go test -count=1 -cover -v $(shell go list ./... | grep -vE '/(e2e|internal/pihole)$$')

container-test:
	go test -count=1 -cover -p 1 -v $(CONTAINER_PKGS)

e2e-test:
	go test -count=1 -cover -v github.com/lovelaze/nebula-sync/e2e

test: unit-test container-test

build: web
	go build .
