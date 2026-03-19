# ⚡ Hermes Scheduler

[![GitHub Tag](https://img.shields.io/github/v/tag/InfyDex/hermes?style=flat-square&label=version)](https://github.com/InfyDex/hermes/tags)
[![Docker Image Size](https://ghcr-badge.egpl.dev/infydex/hermes/size?color=%2344cc11&label=image+size&trim=)](https://github.com/InfyDex/hermes/pkgs/container/hermes)

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
      - "4376:4376"
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
      
      # Optional: Identify which server this instance is running on (used in notification prefixes)
      - HERMES_SERVER_NAME=MyHomeServer
      
      # Optional: Enable clickable links within Discord/Email notifications
      # - HERMES_DOMAIN_URL=https://hermes.example.com
      
      # Optional: Discord webhook configuration
      # - HERMES_DISCORD_WEBHOOK_URL=https://discord.com/api/webhooks/...
      
      # Optional: SMTP Email configuration
      # - HERMES_SMTP_HOST=smtp.gmail.com
      # - HERMES_SMTP_PORT=587
      # - HERMES_SMTP_USER=your-email@gmail.com
      # - HERMES_SMTP_PASS=your-app-password
      # - HERMES_SMTP_FROM=your-email@gmail.com
```

**2. Start the server:**
```bash
docker compose up -d
```

**3. Open the UI:**
Navigate to `http://YOUR_SERVER_IP:4376` and log in with your credentials.

---

## � Notifications & Alerts

Hermes can alert you out-of-the-box whenever jobs fail, hang, or when the application restarts. It supports two main remote delivery methods:

- 🎮 **Discord Webhooks:** Get pinged instantly in your private server. [Read the Discord Setup Guide](docs/discord-setup.md).
- 📧 **SMTP Email:** Deliver logs natively to your inbox (e.g., Gmail). [Read the SMTP Email Setup Guide](docs/email-setup.md).

---

## �🛠️ Usage & Task Runners

When creating a new job, Hermes offers two distinct execution runners depending on your need:

### 1. The Docker Runner (Recommended)
Executes your given command directly by talking to the Docker Daemon securely through the mounted `docker.sock`. Bypasses any shells so stopping/"canceling" a task in Hermes instantly sends the SIGKILL precisely to the Docker process.

**Example Use Cases:**
- Restarting a container: `docker restart jellyfin`
- Running a backup script inside a specific container: `docker exec rclone-runner /scripts/backup.sh`

### 2. The Shell Runner
Throws your command inside an Alpine `sh -c` shell. Required if you need to use boolean operators, piping output, or local variables.

If your command explicitly uses bash syntax, run it with `bash your-script.sh` (the runtime image includes bash).

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
curl -u admin:YOUR_PASSWORD -X POST http://SERVER_IP:4376/api/jobs/2/run
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

## Changelog

Detailed changes for each release are documented in the [CHANGELOG.md](CHANGELOG.md) file.

## License

MIT
