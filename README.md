# WireGate

Self-hosted WireGuard management platform with admin UI.

## Stack

- **Backend:** Go, REST API, OpenAPI
- **Frontend:** React, TypeScript, Vite
- **Database:** SQLite (PostgreSQL optional)
- **Agents:** native Linux / Windows daemons

## Quick Start (Docker)

```bash
git clone https://github.com/zivego/wiregate-releases.git
cd wiregate-releases
docker compose -f deploy/compose/docker-compose.yml up --build -d
```

Open [http://localhost:5173](http://localhost:5173) and log in:

```
admin@example.com / secret
```

To stop:

```bash
docker compose -f deploy/compose/docker-compose.yml down
```

## Local Development

Requires Go and Node.js installed locally.

```bash
make setup   # check prerequisites
make dev     # start backend :8080 + frontend :5173
make stop    # stop
```

## Build

```bash
make build   # produces ./wiregate-server binary
```

## Project Structure

```
cmd/                  server and agent entry points
internal/             backend logic, API, persistence, migrations
pkg/                  WireGuard adapter interfaces
agent/                Linux and Windows agent implementations
web/app/              React frontend
deploy/               Docker, nginx, systemd configs
scripts/              dev, setup, stop helpers
openapi/              API specification
```

