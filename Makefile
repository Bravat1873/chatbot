.PHONY: test build run compose-up compose-down

test:
	go test ./...

build:
	go build ./cmd/server

run:
	go run ./cmd/server

compose-up:
	docker compose up -d --build

compose-down:
	docker compose down
