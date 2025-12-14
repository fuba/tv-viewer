# TV Viewer

A web-based TV viewing application using Mirakurun for Japanese digital broadcasting.

## Features

- Real-time TV streaming with HLS
- ARIB subtitle support (ASS format)
- Channel switching
- Program guide (EPG)
- Web-based UI

## Tech Stack

- Backend: Go
- Frontend: Svelte + TypeScript + Tailwind CSS 
- Streaming: FFmpeg (Docker)
- Database: SQLite
- API: Mirakurun

## Requirements

- Docker & Docker Compose
- Mirakurun server (configured at http://tuner:40772/)

## Quick Start

```bash
docker compose up
```

Access the application at `http://localhost:8080`

## Development

See `doc/Tasks.md` for development roadmap.

## License

CC0 (Creative Commons Zero)