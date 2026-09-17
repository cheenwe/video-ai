.PHONY: install test build dev-backend dev-frontend docker-build docker-up package
install:
	cd frontend && npm ci
	cd backend && go mod download

test:
	cd backend && go test ./... && go vet ./...
	cd frontend && npm run typecheck && npm test

build:
	cd frontend && npm run build
	mkdir -p build
	cd backend && go build -o ../build/baseplatform ./cmd/baseplatform

dev-backend:
	cd backend && go run ./cmd/baseplatform

dev-frontend:
	cd frontend && npm run dev

docker-build:
	docker compose build

docker-up:
	docker compose up -d --build

# Example: make package GOOS=linux GOARCH=arm64
GOOS ?= linux
GOARCH ?= amd64
package:
	cd frontend && npm run build
	bash scripts/package.sh $(GOOS) $(GOARCH)
