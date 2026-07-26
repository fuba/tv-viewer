# Repository Guidelines

## Project Structure & Module Organization

TV Viewer receives Mirakurun streams and delivers them through WebRTC.

- `backend/`: Go server, REST/WebSocket APIs, SQLite, FFmpeg, and WebRTC code under `cmd/` and `internal/`.
- `frontend/`: Svelte 4 + TypeScript UI. Components are in `src/components/`; shared types and WebRTC client code are in `src/lib/`.
- `doc/`: numbered architecture, development, deployment, subtitle, and encoder notes.
- `docker/`, `nginx/`, `docker-compose.yml`, and `docker-compose.dev.yml`: production/development container definitions and proxy configuration.
- `assets/`, `data/`, and `stream/`: fonts and runtime data or stream output; do not commit generated files.

## Build, Test, and Development Commands

Use the `Makefile` for common workflows:

```bash
make deps                         # Download Go and Node dependencies
make test                         # Run all backend Go tests
make dev                          # Run backend and Vite development servers
docker compose -f docker-compose.dev.yml up --build  # Containerized development
make build                        # Build production Docker images
make up                           # Start the production stack
make logs                         # Follow service logs
```

For frontend checks, run `cd frontend && npm run check` and `npm run build`. Production expects Mirakurun and, for GPU encoding, NVIDIA Container Toolkit.

## Coding Style & Naming Conventions

Run `gofmt` on changed Go files and keep packages organized by responsibility under `backend/internal/`. Use idiomatic Go names (`PascalCase` for exported identifiers, `camelCase` for locals). Use two-space indentation in Svelte, TypeScript, and configuration files; name Svelte components in `PascalCase`, and utility/type files descriptively. Keep code comments in English and avoid unrelated formatting changes.

## Testing Guidelines

Backend tests use Go's standard `testing` package and `*_test.go` naming (for example, `backend/internal/webrtc/webrtc_test.go`). Add focused tests for protocol, parser, and session changes, then run `make test`. Frontend uses `npm run check` and `npm run build`; no separate frontend test suite is configured.

## Commit & Pull Request Guidelines

Use short Conventional Commit-style subjects, such as `feat: Add ...`, `fix: Prevent ...`, `docs: Update ...`, or `chore: ...`. Keep commits focused. Pull requests should explain behavior/configuration changes, list validation commands, link issues, and include screenshots or recordings for visible UI changes. Call out Mirakurun, FFmpeg, GPU, or deployment prerequisites.

## Configuration & Security

Copy `.env.example` for local configuration and never commit credentials or private tuner endpoints. Review changes to `docker-compose*.yml`, host networking, exposed ports, and FFmpeg command construction carefully because they affect local infrastructure and stream access.
