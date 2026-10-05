# Learnings

| Date | Task | What happened | How caught | What changed |
| ---- | ---- | ------------- | ---------- | ------------ |
| 2026-10-05 | P0.1 | `go mod edit -go=1.26` is stored as `go 1.26.0`; setup-go reads it from go.mod | go.mod diff | Accepted: the toolchain line pins go1.26.3 |
