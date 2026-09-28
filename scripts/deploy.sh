#!/usr/bin/env bash
#
# Reproducible, SHA-pinned deployment of the Agent Orchestrator on Piave.
#
#   scripts/deploy.sh <git-sha> [git-sha ...]
#
# GitHub is the source of truth: this script fetches from the remote, checks
# out the EXACT commit, builds it, deploys the Quadlet units, restarts the
# services and verifies that /healthz reports the requested commit.
#
# It never uses `git pull` of a moving branch, never copies files from a
# developer machine, and aborts on the first failed step.
#
# Environment overrides:
#   ORCH_REPO_URL   git remote (default: the public GitHub repo)
#   ORCH_SRC        deployment checkout dir (default: ~/agent-orchestrator-src)
#   ORCH_GO_IMAGE   builder image (default: docker.io/library/golang:1.24-alpine)
#   ORCH_HEALTH_URL health endpoint (default: http://127.0.0.1:18080/healthz)
#   ORCH_AUTH       basic auth user:pass for the health endpoint (default: admin:secret)
#   ORCH_UNITS_DIR  Quadlet dir (default: ~/.config/containers/systemd)
#   ORCH_UNIT       orchestrator service unit (default: agent-orchestrator.service)

set -euo pipefail

# Re-exec from a stable copy: the script checks out a different revision of the
# repository mid-run, which would otherwise mutate the file being executed.
if [[ "${ORCH_DEPLOY_REEXEC:-0}" != "1" ]]; then
  TMP="$(mktemp "${TMPDIR:-/tmp}/orch-deploy.XXXXXX.sh")"
  cp "$0" "$TMP"
  ORCH_DEPLOY_REEXEC=1 exec bash "$TMP" "$@"
fi

SHA_ARG="${1:-}"
if [[ -z "$SHA_ARG" ]]; then
  echo "usage: $0 <git-sha>" >&2
  exit 2
fi

REPO_URL="${ORCH_REPO_URL:-https://github.com/stefenellodev-dot/agent-orchestrator.git}"
SRC="${ORCH_SRC:-$HOME/agent-orchestrator-src}"
GO_IMAGE="${ORCH_GO_IMAGE:-docker.io/library/golang:1.24-alpine}"
HEALTH_URL="${ORCH_HEALTH_URL:-http://127.0.0.1:18080/healthz}"
AUTH="${ORCH_AUTH:-admin:secret}"
UNITS_DIR="${ORCH_UNITS_DIR:-$HOME/.config/containers/systemd}"
UNIT="${ORCH_UNIT:-agent-orchestrator.service}"
IMAGE="localhost/agent-orchestrator:latest"
PKG="github.com/stefenello/agent-orchestrator/internal/buildinfo"
GO_HOME="${ORCH_GO_HOME:-$HOME/go-toolchain}"
GO_REQUIRED="go1.27.1"

say() { printf '\n=== %s ===\n' "$*"; }

# Preflight: the pinned Go toolchain is required both to build the binary and to
# give the worker objective self-validation. Fail clearly if it is missing or
# the wrong version (reproducibility guard).
say "preflight: Go toolchain ($GO_REQUIRED)"
GO_PRE="$GO_HOME/go/bin/go"
if [[ ! -x "$GO_PRE" ]]; then
  echo "FAILED: Go toolchain not found at $GO_PRE" >&2
  echo "Bootstrap it first (see docs/deployment-piave.md 'Builder toolchain')." >&2
  exit 1
fi
GO_PRE_VERSION="$("$GO_PRE" version | awk '{print $3}')"
if [[ "$GO_PRE_VERSION" != "$GO_REQUIRED" ]]; then
  echo "FAILED: toolchain is $GO_PRE_VERSION but $GO_REQUIRED is required" >&2
  exit 1
fi
echo "toolchain ok: $GO_PRE ($GO_PRE_VERSION)"

say "source of truth: $REPO_URL"
if [[ -d "$SRC/.git" ]]; then
  git -C "$SRC" remote set-url origin "$REPO_URL"
  git -C "$SRC" fetch --all --tags --prune
else
  git clone "$REPO_URL" "$SRC"
fi

# Resolve the requested ref to a full, immutable SHA.
say "resolve $SHA_ARG"
SHA="$(git -C "$SRC" rev-parse --verify "${SHA_ARG}^{commit}")"
echo "resolved commit: $SHA"
git -C "$SRC" cat-file -e "${SHA}^{commit}"

say "checkout exact SHA (detached)"
git -C "$SRC" checkout --detach --force "$SHA"
git -C "$SRC" clean -fdx

VERSION="$(git -C "$SRC" describe --tags --always || echo "$SHA")"
BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
echo "version: $VERSION  commit: $SHA  build_time: $BUILD_TIME"

say "build binary"
mkdir -p "$SRC/bin"
# Prefer a locally provisioned Go toolchain; fall back to a container builder.
GO_BIN="${ORCH_GO_BIN:-}"
if [[ -z "$GO_BIN" ]]; then
  if [[ -x "$GO_HOME/go/bin/go" ]]; then
    GO_BIN="$GO_HOME/go/bin/go"
  elif command -v go >/dev/null 2>&1; then
    GO_BIN="$(command -v go)"
  fi
fi

LDFLAGS="-s -w -X ${PKG}.Version=${VERSION} -X ${PKG}.Commit=${SHA} -X ${PKG}.BuildTime=${BUILD_TIME}"
if [[ -n "$GO_BIN" ]]; then
  echo "using Go toolchain: $GO_BIN ($("$GO_BIN" version))"
  ( cd "$SRC" && GOTOOLCHAIN=local "$GO_BIN" build -trimpath -ldflags "$LDFLAGS" -o bin/orchestrator ./cmd/orchestrator )
else
  echo "no local Go; using container builder $GO_IMAGE"
  podman run --rm --userns=keep-id \
    -v "$SRC:/src:Z" -w /src \
    -v orchestrator-gomod:/go/pkg/mod \
    -v orchestrator-gocache:/home/builder/.cache/go-build \
    -e HOME=/home/builder -e GOTOOLCHAIN=local \
    "$GO_IMAGE" \
    go build -trimpath -ldflags "$LDFLAGS" -o bin/orchestrator ./cmd/orchestrator
fi

test -x "$SRC/bin/orchestrator" || { echo "binary not produced" >&2; exit 1; }
"$SRC/bin/orchestrator" version

say "build runtime image ($IMAGE)"
podman build -t "$IMAGE" -f "$SRC/configs/quadlet/Containerfile" "$SRC"

say "install Quadlet units"
mkdir -p "$UNITS_DIR"
install -m 0644 "$SRC"/configs/quadlet/*.container "$SRC"/configs/quadlet/*.network "$UNITS_DIR"/

say "daemon-reload + restart"
systemctl --user daemon-reload
systemctl --user restart "$UNIT"

say "health check + SHA verification"
reported=""
for _ in $(seq 1 30); do
  reported="$(curl -sf -u "$AUTH" "$HEALTH_URL" 2>/dev/null \
    | python3 -c 'import sys,json;print(json.load(sys.stdin).get("commit",""))' 2>/dev/null || true)"
  [[ "$reported" == "$SHA" ]] && break
  sleep 2
done

if [[ "$reported" != "$SHA" ]]; then
  echo "FAILED: /healthz reported commit '$reported' but expected '$SHA'" >&2
  exit 1
fi

say "worker toolchain + objective self-validation smoke"
gover="$(podman exec agent-orchestrator go version 2>/dev/null || true)"
echo "worker: ${gover:-<no go>}"
if [[ "$gover" != "go version ${GO_REQUIRED} "* ]]; then
  echo "FAILED: worker 'go version' is '${gover:-none}' (expected $GO_REQUIRED)" >&2
  exit 1
fi
# The toolchain must be mounted read-only inside the worker.
if podman exec agent-orchestrator sh -c 'touch /opt/go-toolchain/.__rwtest' 2>/dev/null; then
  podman exec agent-orchestrator sh -c 'rm -f /opt/go-toolchain/.__rwtest' 2>/dev/null || true
  echo "FAILED: /opt/go-toolchain is writable inside the worker" >&2
  exit 1
fi
echo "toolchain mount is read-only (ok)"
# Run the three objective validations inside the image against the source at SHA.
podman run --rm --entrypoint sh \
  -v "$SRC:/src:ro" \
  -v "$GO_HOME:/opt/go-toolchain:ro" \
  -v orchestrator-gomod:/root/go/pkg/mod \
  -w /src "$IMAGE" \
  -c 'set -e; go version; go vet ./...; go test ./...; go test -race ./...'

say "DEPLOYED $SHA"
curl -s -u "$AUTH" "$HEALTH_URL"; echo
