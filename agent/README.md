# aevora-agent

The node agent runs on each gateway. It self-registers with the control plane,
heartbeats load, and reconciles the WireGuard interface's peers to match the
leases the control plane has issued. It uses a **pull model**: it makes only
outbound calls, accepts no inbound connections, and stores no client private
keys. Peer changes are applied by shelling out to `wg` (installed by the Phase 0
gateway kit in [`infra/gateway`](../infra/gateway)).

## Build

```bash
cd agent
GOOS=linux GOARCH=amd64 go build -o aevora-agent ./cmd/aevora-agent
```

## Run (on a gateway that already has WireGuard up)

The agent registers once (using the enrollment secret), stores the returned node
token at `AEVORA_NODE_TOKEN_FILE`, then loops. Provide the gateway metadata via
environment; the WireGuard public key is read from the live interface.

```bash
export AEVORA_CONTROL_URL=https://control.example.com
export AEVORA_ENROLLMENT_SECRET=...        # only needed for first registration
export AEVORA_WG_INTERFACE=wg0
export AEVORA_GATEWAY_NAME=de-fra-1
export AEVORA_COUNTRY_CODE=de
export AEVORA_COUNTRY_NAME=Germany
export AEVORA_CITY=Frankfurt
export AEVORA_REGION=eu-central
export AEVORA_ENDPOINT_HOST=203.0.113.9     # public IP or hostname
export AEVORA_ENDPOINT_PORT=51820
export AEVORA_WG_SUBNET_V4=10.7.1.0/24
export AEVORA_WG_SUBNET_V6=fd07:0007:1::/64
export AEVORA_CAPACITY=250
export AEVORA_NODE_TOKEN_FILE=/etc/aevora/node.token
export AEVORA_SYNC_INTERVAL=3s
sudo -E ./aevora-agent
```

Run it under systemd on the gateway (a unit ships in a later phase). The
enrollment secret and node token are secrets — keep them out of version control
(the repo `.gitignore` already excludes `*.token` and env files).

## Layout

- `internal/reconcile` — pure current-vs-desired peer diff (unit-tested)
- `internal/wg` — `wg` command wrapper + dump parser (parser unit-tested)
- `internal/cp` — control-plane HTTP client (tested against httptest)
- `cmd/aevora-agent` — the loop: register → heartbeat → fetch peers → reconcile
