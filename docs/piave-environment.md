# Piave Environment Verification

**Status: VERIFIED on the host (2026-09-28).**

| Item | Value |
|------|-------|
| OS | Ubuntu 24.04.4 LTS, x86_64 |
| Podman | 4.9.3 (rootless) |
| systemd | 255.4 (Quadlet supported) |
| Linger | `Linger=yes` |
| sudo | password required (rootless path used instead) |
| Git | 2.43.0 |
| OpenCode | `/home/stefenello/.opencode/bin/opencode` v1.15.12 |
| PostgreSQL | not native — run as a rootless Podman Quadlet container |
| Go | not installed on host — binary cross-compiled on macOS |
| Quadlet dirs | `~/.config/containers/systemd/` (user) |

## Verified commands

```bash
podman --version                                   # 4.9.3
podman info --format '{{.Host.Security.Rootless}}' # true
systemctl --version                                # systemd 255
loginctl show-user stefenello | grep Linger        # Linger=yes
~/.opencode/bin/opencode --version                 # 1.15.12
```

## Decisions recorded

| Decision | Chosen | Rationale |
|----------|--------|-----------|
| Podman mode | **rootless** | No passwordless sudo; user systemd + linger available |
| Quadlet location | `~/.config/containers/systemd/` | user-scoped rootless Quadlet convention |
| SELinux relabel | none needed | Ubuntu uses AppArmor, not SELinux |
| PostgreSQL | container `docker.io/library/postgres:16-alpine` | no native PG on host |
| OpenCode | native binary, bind-mounted into the orchestrator container | public OpenCode container images are not pullable |
| Network | rootless podman network `agent-orchestrator-net` | isolate from other host services |
| Ports | `127.0.0.1:18080` (API) and `127.0.0.1:55432` (PG) | loopback only — no public exposure |

## Note on the OpenCode image

`ghcr.io/sst/opencode` and `docker.io/sst/opencode` are not publicly
pullable (403 / access denied). The deployment therefore runs the verified
native OpenCode binary from `~/.opencode/bin`, bind-mounted read-only into the
orchestrator container at `/opt/opencode`. No plugin or application is installed
inside CondoSmart or fin-engine.
