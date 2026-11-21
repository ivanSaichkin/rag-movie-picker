# make executes simple recipes directly, and in this WSL environment
# /usr/bin/go is a directory that shadows the Go binary during that lookup.
# A shell keeps PATH resolution identical to the terminal.
SHELL := /bin/bash

.DEFAULT_GOAL := check

BINARY := bin/api
GOLANGCI_LINT ?= golangci-lint

.PHONY: check fmt vet test test-race test-integration lint build run tidy docker-build up down logs

check: fmt vet lint test-race

fmt:
	go fmt ./...

vet:
	go vet ./...

test:
	go test ./...

test-race:
	go test -race -count=1 ./...

test-integration:
	go test -race -tags=integration -count=1 ./...

lint:
	$(GOLANGCI_LINT) run ./...

build:
	go build -trimpath -o $(BINARY) ./cmd/api

run:
	go run ./cmd/api

tidy:
	go mod tidy

docker-build:
	docker build -f Dockerfile -t rag-movie-picker-backend:local ..

up:
	docker compose up -d --build

down:
	docker compose down

logs:
	docker compose logs -f api
