.PHONY: test build run compose-up compose-rebuild compose-down deploy

test:
	go test ./...

build:
	go build ./cmd/server

run:
	go run ./cmd/server

compose-up:
	docker compose up -d --build

compose-rebuild:
	docker compose up -d --build app-go

compose-down:
	docker compose down

deploy:
	git pull --ff-only
	docker compose up -d --build --remove-orphans
	docker compose ps
