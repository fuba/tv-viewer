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

# Development mode - run backend and frontend locally
dev:
	@echo "Starting development servers..."
	@make dev-backend & make dev-frontend

dev-backend:
	cd backend && go run cmd/server/main.go

dev-frontend:
	cd frontend && npm install && npm run dev

# Run tests
test:
	cd backend && go test ./...

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
	cd frontend && npm install

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

logs-ffmpeg:
	docker compose logs -f ffmpeg