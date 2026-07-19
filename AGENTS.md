# AGENTS.md

## Cursor Cloud specific instructions

Hermes Scheduler is a single Go service (Go 1.22, CGO) — Web UI, REST API, cron scheduler, executor, and an embedded SQLite DB all live in one binary at `cmd/server`. There is no separate frontend, database, or queue process. Standard build/run details are in `README.md` and `Dockerfile`.

### Building / running the service (dev)

- Build/run require `CGO_ENABLED=1` because of the `mattn/go-sqlite3` driver. `gcc`, `sqlite`, and Go are already available on the VM.
- Non-obvious gotcha: the SQLite DB path (`/data/jobs.db`) and log dir (`/data/logs`) are **hardcoded** in `internal/config/config.go` with no env override. Before running the server, ensure `/data/logs` exists and is writable, e.g. `sudo mkdir -p /data/logs && sudo chown -R "$(id -u):$(id -g)" /data`. This directory lives outside the repo and does not persist to fresh VMs, so recreate it each session.
- Run in dev with: `CGO_ENABLED=1 go run ./cmd/server`. It listens on port `4376` (override with `HERMES_PORT`).
- Auth is HTTP Basic Auth; default credentials are `admin` / `admin` (override with `HERMES_USERNAME` / `HERMES_PASSWORD`). The same credentials protect the Web UI and the REST API.
- The Docker runner (`docker` CLI + `/var/run/docker.sock`), Discord, and SMTP notifications are all optional and not needed to exercise the core product; the Shell runner works with no extra setup.

### Lint / test

- No test suite or linter config exists in the repo. Use `go vet ./...` as the lint check and `go build ./...` to verify compilation.

### Quick end-to-end smoke test

With the server running, create + run a shell job and read its logs via the REST API (Basic Auth):

```
curl -u admin:admin -X POST localhost:4376/api/jobs -H 'Content-Type: application/json' \
  -d '{"name":"smoke","cron_expr":"0 0 * * *","runner_type":"shell","command":"echo hi"}'
curl -u admin:admin -X POST localhost:4376/api/jobs/1/run
curl -u admin:admin localhost:4376/api/jobs/1/executions
curl -u admin:admin localhost:4376/api/executions/1/logs
```
