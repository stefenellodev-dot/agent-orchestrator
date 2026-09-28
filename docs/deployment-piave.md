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
~/go-toolchain/go/               # pinned Go 1.27.1 (build + worker self-validation)
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
1. **Preflight**: require `~/go-toolchain/go` and `go version == go1.27.1`.
2. `git fetch` from GitHub and **verify the SHA exists**.
3. `git checkout --detach <SHA>` (exact commit, `git clean -fdx`).
4. Build the binary with `-ldflags` pinning `Version`/`Commit`=SHA/`BuildTime`.
5. Build the runtime image `localhost/agent-orchestrator:latest`.
6. Install the Quadlet units from the checkout.
7. `systemctl --user daemon-reload` + `restart agent-orchestrator.service`.
8. Poll `/healthz` until the reported `commit` **equals the requested SHA**.
9. **Worker smoke**: `go version` inside the worker == `go1.27.1`, the toolchain
   mount is read-only, and `go vet ./...`, `go test ./...`, `go test -race ./...`
   all exit 0 inside the image.

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

### Worker toolchain (objective self-validation)

Validation commands run **inside the worker container** (`sh -c` in the
worktree, see `internal/service/evidence.go`), not on the host. To let the
orchestrator validate *itself* (`go test ./...`, `go test -race ./...`,
`go vet ./...`) the same pinned toolchain is made available to the worker:

- `configs/quadlet/agent-orchestrator.container` mounts it read-only:
  `Volume=%h/go-toolchain:/opt/go-toolchain:ro`.
- `configs/quadlet/Containerfile` installs `gcc` + `libc6-dev` (required by
  `go test -race`, which needs cgo + a C compiler), adds
  `/opt/go-toolchain/go/bin` to `PATH`, and sets `GOTOOLCHAIN=local` (never
  downloads another toolchain at runtime) and `GOFLAGS=-mod=readonly` (never
  edits `go.mod`/`go.sum` during validation).
- The toolchain is **read-only**; no Podman/Docker socket is added; no extra
  mounts or ports. Isolation of the worker is unchanged.

**Versioned vs operator-managed config.** The repository ships an *example*
(`configs/config.example.yaml`) with the `agent-orchestrator` project's
validation commands. The *effective* project config on Piave lives in the
operator-managed `~/.config/agent-orchestrator/orchestrator.yaml` (not in Git)
and must be kept in sync manually — the deploy never overwrites it.

Manual verification:

```bash
podman exec agent-orchestrator go version                       # go version go1.27.1 linux/amd64
podman exec agent-orchestrator sh -c 'touch /opt/go-toolchain/x' && echo WRITABLE || echo read-only
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
