.PHONY: run test vet fmt build docker-up docker-down docker-reset

run:
	go run ./cmd/server

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

build:
	CGO_ENABLED=0 go build -o bin/plata ./cmd/server

docker-up:
	docker compose up --build -d

docker-down:
	docker compose down

docker-reset:
	docker compose down -v
	docker compose up --build -d
