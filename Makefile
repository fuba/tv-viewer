.PHONY: build up down dev backend frontend test clean

# Build all Docker images
build:
	docker compose build

# Start all services
up:
	docker compose up -d

# Stop all services
down:
	docker compose down

# Development mode uses the production-like Compose environment.
dev:
	docker compose up -d --build backend frontend

dev-backend:
	docker compose up -d --build backend

dev-frontend:
	docker compose up -d --build frontend

# Run tests
test:
	cd backend && go test ./...
	cd frontend && npm test && npm run check

# Clean build artifacts and temp files
clean:
	rm -rf backend/tv-viewer.db
	rm -rf stream/*
	rm -rf frontend/dist
	rm -rf frontend/node_modules
	find . -name "*.log" -delete

# Install dependencies
deps:
	cd backend && go mod download
	cd frontend && npm ci

# Build production images
prod-build:
	docker compose -f docker-compose.yml build --no-cache

# Run in production mode
prod:
	docker compose -f docker-compose.yml up -d

# Show logs
logs:
	docker compose logs -f

# Show logs for specific service
logs-backend:
	docker compose logs -f backend

logs-frontend:
	docker compose logs -f frontend
