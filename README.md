# komposer

TUI docker-compose.yml generator. Build and configure services (image, ports,
env, volumes, depends_on) in a 3-pane terminal UI with a live YAML preview,
then export straight to `docker-compose.yml`.

## Status

Work in progress, built in phases:

- [x] Phase 1 — project setup & domain model (`pkg/composer`)
- [x] Phase 2 — base 3-pane UI layout (Lipgloss)
- [x] Phase 3 — state management & navigation (add/delete, form editing, scrollable preview)
- [x] Phase 4 — presets & file saving (PostgreSQL, Redis, Nginx, MySQL, MongoDB)
- [ ] Phase 5 — validation & import

## Requirements

- Go 1.22+

## Setup

```sh
go mod tidy
go build -o komposer.exe .
```

## Run

```sh
./komposer.exe
```

## Keybindings

### Navigation
- `Tab` / `Shift+Tab` — switch focus between panes
- `↑` / `↓` or `k` / `j` — navigate services (left pane) or scroll YAML (right pane)
- `PgUp` / `PgDown` — scroll YAML preview (right pane)

### Services
- `a` — add new service (prompts for name)
- `d` — delete selected service (prompts for confirmation)
- `Enter` or `e` — edit selected service (center pane)

### Form Editing
- `Tab` / `Shift+Tab` — next/previous field
- `Enter` or `Esc` — save and exit edit mode

### Presets & Saving
- `Ctrl+P` — open preset picker (PostgreSQL, Redis, Nginx, MySQL, MongoDB)
- `Ctrl+S` — save `docker-compose.yml`

### General
- `q` / `Ctrl+C` — quit
