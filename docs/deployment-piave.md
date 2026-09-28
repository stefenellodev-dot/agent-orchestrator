# Piave Deployment Runbook

Target: a Linux host running **Podman + Quadlet/systemd**. No Kubernetes, no
managed infrastructure, no message broker.

> **Verification status:** the artifacts below are prepared but the end-to-end
> Piave run is **PENDING**. Complete `docs/piave-environment.md` first.

## 1. Host layout

```
/opt/agent-orchestrator/
├── bin/orchestrator              # single static Go binary
└── docs/                         # this runbook, capabilities, env verification
/etc/agent-orchestrator/
├── orchestrator.yaml             # config (see configs/config.example.yaml)
└── secrets.env                   # 0600, owned by orchestrator
/var/lib/agent-orchestrator/
├── worktrees/                    # one worktree per WorkItem
└── (postgres data if local)
```

## 2. Service user

```bash
sudo useradd --system --home /var/lib/agent-orchestrator --shell /usr/sbin/nologin orchestrator
sudo install -d -o orchestrator -g orchestrator /var/lib/agent-orchestrator/worktrees
sudo install -d -o orchestrator -g orchestrator /etc/agent-orchestrator
```

## 3. Secrets

`/etc/agent-orchestrator/secrets.env` (chmod 600, chown orchestrator):

```
OPENCODE_API_KEY=...
# Provider credentials used by `opencode run`
DATABASE_URL=postgres://orchestrator:...@localhost:5432/orchestrator?sslmode=disable
```

Secrets are injected as environment variables only. They are never written into
a worktree or committed.

## 4. Build and install

```bash
make build            # bin/orchestrator
make web-build        # builds web/ and refreshes internal/web/dashboard
sudo install -m 0755 bin/orchestrator /opt/agent-orchestrator/bin/orchestrator
```

## 5. Database

```bash
/opt/agent-orchestrator/bin/orchestrator migrate --config /etc/agent-orchestrator/orchestrator.yaml
```

Idempotent; safe to run on every deploy. Set `store.kind: postgres` in the config.

## 6. systemd units

```bash
sudo cp configs/systemd/orchestrator.service     /etc/systemd/system/
sudo cp configs/systemd/orchestrator-web.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now orchestrator.service orchestrator-web.service
```

## 7. Quadlet worker

```bash
sudo cp configs/quadlet/opencode-worker@.container /etc/containers/systemd/
sudo systemctl daemon-reload
# One instance is launched per phase; see the comment header in the .container file.
```

## 8. Per-project configuration

Project definitions live in `/etc/agent-orchestrator/orchestrator.yaml`
(orchestrator-owned; consumer repos need no changes):

```yaml
projects:
  - name: condosmart
    repo_path: /srv/repos/condosmart
    base_branch: main
    validation_commands: ["npm test"]
    lint_commands: ["npm run lint"]
    typecheck_commands: ["npm run typecheck"]
```

## 9. Verification checklist (must pass before real use)

- [ ] `systemctl status orchestrator` — active (running)
- [ ] `curl -u admin:... https://host/healthz` — `{"status":"ok"}`
- [ ] Open the dashboard, create a WorkItem against a **test repo**
- [ ] It pauses at `awaiting_approval`
- [ ] Approve in the dashboard → runs to `complete`
- [ ] `journalctl -u orchestrator -n 100` shows the phase transitions
- [ ] Worktree removed on `complete`; retained on `failed`/`blocked`
- [ ] No secrets present anywhere under `/var/lib/agent-orchestrator/worktrees`

## 10. Operations

```bash
journalctl -u orchestrator.service -f
systemctl restart orchestrator.service
# Inspect retained worktrees for a failed WorkItem:
ls /var/lib/agent-orchestrator/worktrees/
```

## Rollback

The orchestrator is stateless; roll back by installing the previous binary and
restarting the unit. Database migrations are additive and idempotent.
