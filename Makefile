APP=hexsonic

.PHONY: build build-api build-worker run-api run-worker test lint up down

build: build-api build-worker

build-api:
	go build -o bin/hexsonic-api ./cmd/hexsonic-api

build-worker:
	go build -o bin/hexsonic-worker ./cmd/hexsonic-worker

run-api:
	go run ./cmd/hexsonic-api

run-worker:
	go run ./cmd/hexsonic-worker

test:
	go test ./...

lint:
	go vet ./...

up:
	docker compose up -d --build

down:
	docker compose down
