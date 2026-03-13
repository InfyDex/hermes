# ⚡ Hermes Scheduler

A beautifully lightweight, self-hosted cron scheduler with a simple Web UI and direct Docker integration. Built perfectly for OpenMediaVault, Unraid, Raspberry Pis, and single-server homelab environments.

![Hermes Dashboard Preview](assets/icons/hermes.png) <!-- Will render the logo if placed in the root's `assets/` relative path, or you can update with an actual screenshot! -->

Say goodbye to messy host-machine `crontab` files, and manage all your automated server tasks, backups, and container reboots from one clean dashboard.

## ✨ Features

- **No Database Overhead:** Completely self-contained embedding its own high-performance SQLite database.
- **Visual Schedule Builder:** No more struggling to remember cron syntax! Use the intuitive UI to map out hourly, daily, weekly, or specific interval schedules.
- **Docker Socket Integration:** Talk natively to your host's Docker daemon. Restart containers or trigger scripts *inside* other containers natively from the UI.
- **Live Output Logs:** Watch your jobs execute in real-time right from the browser. Logs are cleanly rotated to plaintext files on disk.
- **Overlap Prevention & Timeouts:** Prevent scripts from piling up by forcing single-threaded runs, or enforce hard time limits on hanging jobs.
- **Micro Footprint:** A statically compiled Go binary running inside a ~15MB Alpine Docker container. Drops memory usage to virtually 0 at idle.

---

## 🚀 Quick Start (Docker Compose)

The easiest way to run Hermes is via Docker Compose using the highly optimized multi-architecture image from the GitHub Container Registry.

**1. Create a `docker-compose.yml` file:**
```yaml
services:
  hermes:
    image: ghcr.io/infydex/hermes:latest
    container_name: hermes
    restart: unless-stopped
    ports:
      - "8080:8080"
    volumes:
      # Data storage (Jobs DB and log files)
      - /path/to/your/appdata/hermes:/data
      
      # [Optional] Allows Hermes to run commands on other containers!
      - /var/run/docker.sock:/var/run/docker.sock
    environment:
      - TZ=Asia/Kolkata
      
      # Optional Override: Change the default 'admin' login credentials
      - HERMES_USERNAME=admin
      - HERMES_PASSWORD=admin
```

**2. Start the server:**
```bash
docker compose up -d
```

**3. Open the UI:**
Navigate to `http://YOUR_SERVER_IP:8080` and log in with your credentials.

---

## 🛠️ Usage & Task Runners

When creating a new job, Hermes offers two distinct execution runners depending on your need:

### 1. The Docker Runner (Recommended)
Executes your given command directly by talking to the Docker Daemon securely through the mounted `docker.sock`. Bypasses any shells so stopping/"canceling" a task in Hermes instantly sends the SIGKILL precisely to the Docker process.

**Example Use Cases:**
- Restarting a container: `docker restart jellyfin`
- Running a backup script inside a specific container: `docker exec rclone-runner /scripts/backup.sh`

### 2. The Shell Runner
Throws your command inside an Alpine `sh -c` shell. Required if you need to use boolean operators, piping output, or local variables.

**Example Use Case:**
- Chaining updates: `docker restart nginx && echo "Ping successful" > /tmp/ping.txt`

---

## ⚙️ Advanced Configuration (API)

Hermes boasts a fully capable REST API to trigger jobs externally (from things like GitHub Actions, Uptime Kuma webhooks, or n8n).

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/jobs/{id}/run` | Remotely trigger a job to run right now |
| POST | `/api/executions/{id}/cancel` | Abort a running execution |
| GET | `/api/jobs` | Dump list of all configured jobs |
| GET | `/api/executions/{id}/logs` | Fetch the raw log stream |

*All endpoints require HTTP Basic Auth using your configured username and password.*

**Trigger a job remotely:**
```bash
curl -u admin:YOUR_PASSWORD -X POST http://SERVER_IP:8080/api/jobs/2/run
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
