# Piave Environment Verification

**Status: PENDING — must be executed on the Piave host.**

This document is the blocking gate for Milestone 5. The orchestrator was built
and tested locally on macOS (darwin/arm64), where `systemd` and `podman` are
not available. The commands below must be run on Piave and their output
recorded before deploying.

## Commands to run on Piave

```bash
cat /etc/os-release
uname -a
podman --version
systemctl --version
loginctl show-user "$USER" | grep Linger
sysctl kernel.unprivileged_userns_clone 2>/dev/null || echo "n/a"
getenforce 2>/dev/null || echo "no selinux"
podman info --format '{{.Host.Security.Rootless}}'
```

## Decisions to record

| Decision | Options | Chosen | Notes |
|----------|---------|--------|-------|
| Podman mode | rootless / rootful | TBD | Rootless preferred if user namespaces + linger available |
| Linger | enabled / disabled | TBD | Required for rootless user services to survive logout |
| SELinux relabel | `:Z` / `:z` / none | TBD | Depends on `getenforce` |
| systemd version | — | TBD | Needs `Type=exec`, `EnvironmentFile`, Quadlet support (v252+) |
| Worktree storage path | — | TBD | Default `/var/lib/agent-orchestrator/worktrees` |

## Local environment (for reference, NOT Piave)

| Item | Value |
|------|-------|
| OS | darwin/arm64 |
| podman | not installed |
| systemd | not available |
| PostgreSQL | 14.18 (Homebrew) |

## Why this is pending

The MVP was validated end-to-end locally (Milestones 0–4) using the local
`opencode run` path. The Piave-specific path swaps only the process launcher;
its artifacts are prepared but cannot be verified without host access.
