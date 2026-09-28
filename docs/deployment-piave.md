# Piave Deployment (Podman + Quadlet + systemd, rootless)

Deployed and verified 2026-09-28. No Kubernetes, no broker, no managed infra.

## Topology

```
                  Piave (Ubuntu 24.04, rootless podman + user systemd)
  ┌────────────────────────────────────────────────────────────────────┐
  │ agent-orchestrator.container        agent-orchestrator-postgres    │
  │  image localhost/agent-orchestrator  image postgres:16-alpine      │
  │  API+dashboard+OpenCode worker       data: ~/podman-data/...       │
  │  127.0.0.1:18080 -> 8080             127.0.0.1:55432 -> 5432        │
  │         │                                     ▲                    │
  │         └──────── agent-orchestrator-net ─────┘                    │
  └────────────────────────────────────────────────────────────────────┘
```

## Layout on Piave

```
~/agent-orchestrator/
  bin/orchestrator        # cross-compiled linux/amd64 binary
  Containerfile           # builds localhost/agent-orchestrator:latest
~/.config/agent-orchestrator/
  orchestrator.yaml       # config (projects, DSN, agents, model)
  secrets.env             # 0600
~/.local/share/agent-orchestrator/
  worktrees/              # one worktree per WorkItem
  repos/                  # orchestrator-accessible repositories
~/.config/containers/systemd/
  agent-orchestrator-net.network
  agent-orchestrator-postgres.container
  agent-orchestrator.container
```

## Deploy / update

```bash
# On macOS: build linux binary with identity
make build-linux
scp bin/orchestrator-linux-amd64 piave:~/agent-orchestrator/bin/orchestrator

# On Piave
ssh piave
cd ~/agent-orchestrator && podman build -t localhost/agent-orchestrator:latest .
systemctl --user daemon-reload
systemctl --user restart agent-orchestrator.service
```

## Operations

```bash
systemctl --user status agent-orchestrator.service
systemctl --user status agent-orchestrator-postgres.service
journalctl --user -u agent-orchestrator.service -f
curl -s -u admin:secret http://127.0.0.1:18080/healthz   # exposes build identity
podman ps --filter name=agent-orchestrator
```

## Persistence

PostgreSQL data lives in `~/podman-data/agent-orchestrator-postgres`. WorkItem
state, sessions, gates and events survive `systemctl --user restart`.

## Identity guarantee

`/healthz` returns `version`, `commit`, `build_time` and the detected
`opencode` version, so an obsolete binary can never run silently.

## Secrets

Provider credentials are supplied via the bind-mounted
`~/.local/share/opencode/auth.json` (`:rw` for session state). No secrets are
written into worktrees or consumer repositories.
