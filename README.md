# Hermes

Lightweight self-hosted cron scheduler with a web UI. Designed for homelab and single-server environments.

## Features

- Cron scheduling with standard expressions (6-field: sec min hour day month weekday)
- Shell and Docker command execution
- Job execution history with log capture
- Overlap prevention (configurable per job)
- Timeout support
- Web UI for job management
- REST API for integrations
- Basic authentication
- SQLite persistence
- Single binary, Docker-ready

## Quick Start

### Docker Compose (recommended)

```bash
docker compose up -d
```

Open http://localhost:8080 — login with `admin` / `admin`.

### Build from source

```bash
# Requires Go 1.22+ and CGO (for SQLite)
go build -o hermes ./cmd/server
mkdir -p /data/logs
./hermes -config config.yaml
```

## Configuration

Edit `config.yaml`:

```yaml
server:
  port: 8080

auth:
  username: admin
  password: changeme

database:
  path: /data/jobs.db

logs:
  directory: /data/logs
```

## Docker Integration

Mount the Docker socket to allow executing Docker commands from scheduled jobs:

```yaml
volumes:
  - /var/run/docker.sock:/var/run/docker.sock
```

Example job command:

```
docker exec nextcloud php cron.php
```

## REST API

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/jobs` | List all jobs |
| POST | `/api/jobs` | Create a job |
| GET | `/api/jobs/{id}` | Get job details |
| PUT | `/api/jobs/{id}` | Update a job |
| DELETE | `/api/jobs/{id}` | Delete a job |
| POST | `/api/jobs/{id}/run` | Trigger manual run |
| GET | `/api/jobs/{id}/executions` | List executions |
| POST | `/api/executions/{id}/cancel` | Cancel running execution |
| GET | `/api/executions/{id}/logs` | Get execution logs |

All endpoints require Basic Auth.

### Create a job via API

```bash
curl -u admin:admin -X POST http://localhost:8080/api/jobs \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "Hello World",
    "cron_expr": "0 */5 * * * *",
    "command": "echo hello",
    "runner_type": "shell"
  }'
```

## Project Structure

```
cmd/server/          Main entry point
internal/
  api/               REST API handlers + auth middleware
  config/            Configuration loading
  database/          SQLite persistence layer
  executor/          Job execution engine
  models/            Data models
  runners/           Pluggable runner interface + implementations
  scheduler/         Cron scheduler
  web/               Web UI handler + templates
```

## Architecture

```
┌──────────────────────────────────┐
│          HTTP Server             │
│  ┌──────────┐  ┌──────────────┐ │
│  │  Web UI  │  │   REST API   │ │
│  └────┬─────┘  └──────┬───────┘ │
│       │               │         │
│  ┌────┴───────────────┴───────┐ │
│  │        Scheduler           │ │
│  └────────────┬───────────────┘ │
│               │                 │
│  ┌────────────┴───────────────┐ │
│  │        Executor            │ │
│  └────────────┬───────────────┘ │
│               │                 │
│  ┌────────────┴───────────────┐ │
│  │    Runner Registry         │ │
│  │  ┌───────┐  ┌──────────┐  │ │
│  │  │ Shell │  │  Docker  │  │ │
│  │  └───────┘  └──────────┘  │ │
│  └────────────────────────────┘ │
│               │                 │
│  ┌────────────┴───────────────┐ │
│  │     SQLite Database        │ │
│  └────────────────────────────┘ │
└──────────────────────────────────┘
```

## Adding New Runners

Implement the `Runner` interface:

```go
type Runner interface {
    Type() models.RunnerType
    Execute(ctx context.Context, job *models.Job, output io.Writer) (exitCode int, err error)
}
```

Register in `cmd/server/main.go`:

```go
registry.Register(myNewRunner)
```

## License

MIT
