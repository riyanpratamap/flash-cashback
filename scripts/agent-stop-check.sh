#!/usr/bin/env bash
# Claude Code Stop / SubagentStop hook: an agent may finish only when the fast checks pass.
# Exit 0 lets it finish. Exit 2 blocks it and sends stderr back to the agent to fix.
input=$(cat)

# Loop guard: after one forced retry, let the agent stop (its report must then start with FAILED).
if printf '%s' "$input" | grep -Eq '"stop_hook_active"[[:space:]]*:[[:space:]]*true'; then exit 0; fi

cd "$(git rev-parse --show-toplevel 2>/dev/null || pwd)" || exit 0

changed=$( { git diff --name-only HEAD 2>/dev/null; git ls-files --others --exclude-standard 2>/dev/null; } )
fail=0
out=""

# Go half: active once backend/go.mod exists, and only when Go files changed.
if [ -f backend/go.mod ] && printf '%s\n' "$changed" | grep -Eq '^backend/.*\.go$|^backend/go\.(mod|sum)$'; then
  unformatted=$(cd backend && gofmt -l . 2>&1)
  if [ -n "$unformatted" ]; then
    fail=1
    out="$out
gofmt: these files are not formatted (run gofmt -w):
$unformatted"
  fi
  if ! vet=$(cd backend && go vet ./... 2>&1); then
    fail=1
    out="$out
go vet failed:
$vet"
  fi
fi

# TypeScript half: active once mobile/package.json defines lint and typecheck, and only when TS changed.
if [ -f mobile/package.json ] && printf '%s\n' "$changed" | grep -Eq '^mobile/.*\.(ts|tsx)$'; then
  if (cd mobile && node -e 'const s=require("./package.json").scripts||{};process.exit(s.lint&&s.typecheck?0:1)' 2>/dev/null); then
    if ! ts=$(cd mobile && { npm run --silent lint && npm run --silent typecheck; } 2>&1); then
      fail=1
      out="$out
mobile lint or typecheck failed:
$ts"
    fi
  fi
fi

[ "$fail" -eq 0 ] && exit 0
{
  echo "Stop check failed. Fix the cause; never disable a rule or add an ignore."
  printf '%s\n' "$out" | tail -n 60
} >&2
exit 2
