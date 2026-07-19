# Fleet Setup Guide

Hermes Fleet lets multiple Hermes instances discover each other, track online/offline status, and send alerts when a peer goes down. Fleet is **optional** — a single Hermes node works exactly as before with zero fleet configuration.

## Single node vs multi-node

| Setup | What you need |
|-------|----------------|
| **One server** | Nothing. Fleet UI is available but peers list stays empty. |
| **Two or more servers** | Pair each Hermes instance from the dashboard using peer secrets. |

There is no fixed limit on the number of nodes. Each instance only tracks peers you explicitly connect.

## Prerequisites

1. Each Hermes instance must be reachable from the others over HTTP/HTTPS.
2. Set `HERMES_DOMAIN_URL` on **each** node to its public URL (required for handshake callbacks behind NAT or reverse proxy).
3. Optionally set `HERMES_NODE_ID` for a stable node identifier (defaults to `HERMES_SERVER_NAME` or hostname).

## Step-by-step pairing

**On Node A:**

1. Open **Fleet Settings** (`/fleet`).
2. Copy the **Peer secret**.
3. Note the **Public URL** (from `HERMES_DOMAIN_URL`).

**On Node B:**

1. Open the **Dashboard**.
2. Expand **Add peer node**.
3. Enter:
   - **Display name:** e.g. `backup-node`
   - **Peer URL:** Node A's URL (e.g. `https://hermes-a.example.com:4376`)
   - **Remote peer secret:** paste Node A's secret
4. Click **Connect Peer**.

Node B performs a handshake with Node A. Both nodes register each other. Within ~30 seconds, each dashboard should show the peer as **online**.

Repeat for additional nodes (pairwise). For a full mesh of N nodes, connect each pair once from either side.

## Environment variables

| Variable | Required | Description |
|----------|----------|-------------|
| `HERMES_DOMAIN_URL` | Recommended for fleet | This node's public base URL (e.g. `https://hermes.example.com`) |
| `HERMES_NODE_ID` | No | Stable node ID used in fleet API (fallback: `HERMES_SERVER_NAME`, then hostname) |
| `HERMES_SERVER_NAME` | No | Display name in notifications and default fleet node name |

## API reference

### Public

```
GET /api/health
→ { "status": "ok", "node_id": "my-node" }
```

### Peer authentication (Bearer token)

Use the **target node's peer secret** as `Authorization: Bearer <secret>`.

```
POST /api/fleet/handshake
{
  "node_id": "node-a",
  "name": "Node A",
  "address": "https://hermes-a.example.com",
  "peer_secret": "<node-a-secret-for-callback>"
}
→ { "node_id": "node-b", "name": "Node B", "address": "https://hermes-b.example.com" }

POST /api/fleet/heartbeat
{
  "node_id": "node-a",
  "name": "Node A",
  "address": "https://hermes-a.example.com"
}
→ 204 No Content
```

### Admin (session or Basic Auth)

```
GET  /api/fleet/peers
GET  /api/fleet/local
POST /api/fleet/peers        { "name", "address", "peer_secret" }
PUT  /api/fleet/local        { "name", "regenerate_secret": true }
DELETE /api/fleet/peers/{id}
```

## Docker: two-node local test

```yaml
services:
  hermes-a:
    image: ghcr.io/infydex/hermes:latest
    ports: ["4376:4376"]
    environment:
      - HERMES_SESSION_SECRET=change-me-to-a-random-64-char-hex-string-at-least-32-bytes
      - HERMES_NODE_ID=node-a
      - HERMES_DOMAIN_URL=http://localhost:4376
    volumes:
      - ./data-a:/data

  hermes-b:
    image: ghcr.io/infydex/hermes:latest
    ports: ["4377:4376"]
    environment:
      - HERMES_SESSION_SECRET=change-me-to-a-random-64-char-hex-string-at-least-32-bytes
      - HERMES_NODE_ID=node-b
      - HERMES_DOMAIN_URL=http://localhost:4377
    volumes:
      - ./data-b:/data
```

1. Start both containers.
2. Open `http://localhost:4376/fleet` → copy peer secret.
3. Open `http://localhost:4377` → add peer with URL `http://host.docker.internal:4376` (or host IP) and Node A's secret.

## Troubleshooting

| Problem | Fix |
|---------|-----|
| Handshake failed | Check URL, firewall, and that the remote peer secret is correct |
| Peer stays offline | Verify `HERMES_DOMAIN_URL` on both nodes; check heartbeat can reach `/api/fleet/heartbeat` |
| Cannot add peer | Set `HERMES_DOMAIN_URL` on the node doing the add |
| Secret leaked | Regenerate in Fleet Settings and re-pair all peers |

## Security notes

- Peer secrets are stored in the local SQLite database (plaintext in Phase 1).
- Use HTTPS for internet-facing nodes.
- Peer API auth is separate from admin login credentials.
- Do not expose Hermes to the public internet without changing default credentials.

## Future: folder sync

Fleet Phase 1 does not transfer files or run remote commands. Until remote execution ships, you can use a **shell job** with `rsync -e ssh` on the scheduling node. The fleet panel shows whether the peer host is reachable.
