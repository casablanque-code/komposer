# komposer

TUI docker-compose.yml generator. Build and configure services (image, ports,
env, volumes, depends_on) in a 3-pane terminal UI with a live YAML preview,
then export straight to `docker-compose.yml`.

## Status

Work in progress, built in phases:

- [x] Phase 1 — project setup & domain model (`pkg/composer`)
- [ ] Phase 2 — base 3-pane UI layout (Lipgloss)
- [ ] Phase 3 — state management & navigation
- [ ] Phase 4 — live preview
- [ ] Phase 5 — presets & file saving

## Requirements

- Go 1.22+

## Setup

```sh
go mod tidy   # fetches bubbletea, lipgloss, bubbles, yaml.v3 and writes go.sum
go build ./...
```

> go.sum is not checked in yet — this repo was assembled without network
> access, so `go mod tidy` needs to run once locally to resolve and pin
> dependency checksums before the module will build.

## Run

```sh
go run .
```

## Keybindings (planned)

- `Tab` / `Shift+Tab` — switch focus between panes
- `a` — add a new service
- `d` — delete selected service
- `Ctrl+P` — open preset picker (Postgres, Redis, Nginx, ...)
- `Ctrl+S` — save `docker-compose.yml`
- `q` / `Ctrl+C` — quit
