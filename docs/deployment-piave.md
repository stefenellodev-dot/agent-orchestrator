# Piave Deployment (Podman + Quadlet + systemd, rootless) — GitOps by SHA

Deployed and verified 2026-09-28. **GitHub is the source of truth.** No Kubernetes,
no broker, no scp/rsync from a developer machine.

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

## Piave layout

```
~/agent-orchestrator-src/        # deployment checkout (source of truth: GitHub)
~/.config/agent-orchestrator/    # orchestrator.yaml + secrets.env (operator-managed)
~/.local/share/agent-orchestrator/{worktrees,repos}
~/podman-data/agent-orchestrator-postgres/
~/.config/containers/systemd/    # Quadlet units (installed from the checkout)
  agent-orchestrator-net.network
  agent-orchestrator-postgres.container
  agent-orchestrator.container
```

Note: the environment config (`orchestrator.yaml`, `secrets.env`) is
operator-managed and intentionally **not** in Git (it holds host paths and
secrets). Everything else — code, Containerfile, Quadlet units, deploy script —
comes from the repository.

## Development → commit → push (Mac)

```bash
make build            # local build
ORCHESTRATOR_TEST_DSN=... go test -race -count=1 ./...
go vet ./...
git add -A && git commit -m "..."
git push origin main  # HTTPS via gh credential helper
```

Record the pushed SHA (`git rev-parse HEAD`).

## Deploy an exact SHA (Piave)

```bash
ssh piave
~/agent-orchestrator-src/scripts/deploy.sh <SHA>
```

What it does (fails fast on any error):
1. `git fetch` from GitHub and **verify the SHA exists**.
2. `git checkout --detach <SHA>` (exact commit, `git clean -fdx`).
3. Build the binary in a Go container with `-ldflags` pinning `Version`/`Commit`=SHA/`BuildTime`.
4. Build the runtime image `localhost/agent-orchestrator:latest`.
5. Install the Quadlet units from the checkout.
6. `systemctl --user daemon-reload` + `restart agent-orchestrator.service`.
7. Poll `/healthz` until the reported `commit` **equals the requested SHA**.

It never `git pull`s a moving branch and never copies files from a developer
machine.

### Builder toolchain

Piave has no system Go and the Docker Hub Go images pull prohibitively slowly
on its network, so the deploy uses a **user-provisioned Go toolchain** at
`~/go-toolchain/go/bin/go` (the Go 1.27.1 linux/amd64 toolchain module fetched
from `proxy.golang.org`). Override with `ORCH_GO_BIN=/path/to/go`. If no local
Go is found, the script falls back to a container builder (`ORCH_GO_IMAGE`).

Bootstrap (one time, on Piave):

```bash
mkdir -p ~/go-toolchain && cd ~/go-toolchain
curl -sSL -o go.zip "https://proxy.golang.org/golang.org/toolchain/@v/v0.0.1-go1.27.1.linux-amd64.zip"
python3 -c 'import zipfile;zipfile.ZipFile("go.zip").extractall(".")'
mv golang.org/toolchain@v0.0.1-go1.27.1.linux-amd64 go
rm -rf go.zip golang.org
~/go-toolchain/go/bin/go version
```

## Validate

```bash
curl -s -u admin:secret http://127.0.0.1:18080/healthz
# { "version": ..., "commit": "<SHA>", "build_time": ..., "opencode": "1.15.12" }
systemctl --user status agent-orchestrator.service
systemctl --user status agent-orchestrator-postgres.service
journalctl --user -u agent-orchestrator.service -n 30
```

## Rollback

Rollback is just a deployment of a previous SHA — no source edits required:

```bash
~/agent-orchestrator-src/scripts/deploy.sh <previous-SHA-or-tag>
```

Because every image is tagged and the deploy rebuilds from the exact commit,
selecting an older SHA restores that exact build. PostgreSQL data is never
touched by a rollback (`agent-orchestrator-postgres` is not restarted).

## Persistence

PostgreSQL data lives in `~/podman-data/agent-orchestrator-postgres`. WorkItem
state, sessions, gates and events survive `systemctl --user restart
agent-orchestrator.service`.

## Identity guarantee

`/healthz` returns `version`, `commit`, `build_time` and the detected
`opencode` version, so an obsolete binary can never run silently.

## Secrets

Provider credentials come from the bind-mounted
`~/.local/share/opencode/auth.json`. No secrets are written into worktrees or
into consumer repositories.

Basic-Auth credentials for the dashboard/API can be supplied from the
environment (systemd `EnvironmentFile`), so the real password never lives in
Git. Environment overrides the YAML; an empty variable falls back to the YAML:

```
# ~/.config/agent-orchestrator/secrets.env  (chmod 600, NOT in Git)
ORCHESTRATOR_AUTH_USERNAME=stefenello
ORCHESTRATOR_AUTH_PASSWORD=<set by the administrator>
```

Recognized overrides: `ORCHESTRATOR_AUTH_USERNAME`, `ORCHESTRATOR_AUTH_PASSWORD`,
`ORCHESTRATOR_AUTH_ENABLED`. The example config keeps only clearly-documented
development credentials (`admin` / `changeme`).
