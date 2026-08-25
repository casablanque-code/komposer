# komposer

A terminal UI for putting together a `docker-compose.yml` fast — either
service by service, or as a ready-made multi-service stack (a database
plus its admin UI, a blog platform plus its database, and so on) in a
single keystroke. Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea)
and [Lipgloss](https://github.com/charmbracelet/lipgloss).

It also validates what you build as you go — not just YAML syntax, but
common Docker footguns: ports quietly published on every network
interface, an empty or hardcoded secret, a database with no volume
(so its data disappears the moment the container is recreated).

It's also safe to use as a viewer/editor on a `docker-compose.yml` you
already have. Import keeps every field verbatim, including ones this
tool's form can't edit (`command`, `networks`, `container_name`,
`labels`, `env_file`, `secrets`, `x-*` extensions, and so on) — editing
one service through the form and saving never drops the rest of the
file.

## Features

- **3-pane TUI**: services list, service config, live YAML preview —
  the preview updates as you type, so you always see the file you're
  about to save.
- **First-launch screen**: an empty config shows a small button menu
  (add a service / browse presets & stacks / import a file) instead of
  three empty panes — navigable by arrow keys, Enter, or a mouse click.
- **Presets** — drop in a single well-configured service (Postgres,
  Redis, Nginx, MySQL, MongoDB) instead of typing out `image:`,
  `ports:`, etc. by hand.
- **Stacks** — drop in several services at once, already wired
  together (`depends_on`, matching hostnames, shared env vars). Covers
  common combinations like a database with its admin UI (Postgres +
  PgAdmin, MySQL + phpMyAdmin, Mongo + Mongo Express, Adminer +
  Postgres), a CMS or app platform with its database (WordPress +
  MySQL, Ghost + MySQL, Nextcloud + Postgres, Nextcloud + Redis +
  MariaDB, Directus + Postgres), and common infra pairings
  (Prometheus + Grafana, Elasticsearch + Logstash + Kibana, Redis +
  RedisInsight, Gitea + Postgres, Metabase + Postgres, n8n + Postgres,
  Portainer + Watchtower). If a stack's default service name is
  already taken, it's renamed automatically (`db` → `db-2`) rather
  than refusing or silently overwriting anything.
- **Validation** (`Ctrl+V`, scrollable) — checks required fields, port
  formats, restart policies, and `depends_on` references, and
  separately flags non-blocking advisory warnings:
  - a port published on every network interface (Docker's default)
    with a suggested `127.0.0.1:...` fix,
  - an environment variable that looks like a secret (by name) with an
    empty or hardcoded value,
  - a recognized database/stateful image with no volume configured,
  - Postgres published on the network with no `POSTGRES_PASSWORD` set.

  Warnings never block saving — they're advisory, shown separately
  from hard errors.
- **Import** (`Ctrl+O`) an existing `docker-compose.yml` — parses the
  raw YAML tree rather than a fixed struct, so any field this tool
  doesn't have explicit support for is captured and re-emitted
  untouched on export instead of being silently dropped. Both
  `depends_on` forms (short list and long map-with-condition) are
  understood.
- **Explicit save** (`Ctrl+S`) — prompts for a path (defaulting to
  `docker-compose.yml`) and asks before overwriting an existing file.
  If you quit (`q`) with unsaved changes, you get the same prompt
  instead of losing them silently.
- **Editing a service**: Esc discards your edits (asking for
  confirmation only if you actually changed something) rather than
  saving automatically; `Ctrl+S` saves explicitly.
- Mouse wheel and keyboard both scroll the services list, the YAML
  preview, and the validation report.

## Requirements

- Go 1.24.2+

## Build

```sh
go build -o komposer .
```

Or run it directly without a separate build step:

```sh
go run .
```

## Usage

```sh
./komposer
```

### First launch (no services yet)

| Key | Action |
|---|---|
| `↑` / `↓` | move between the 3 buttons |
| `Enter`, or a mouse click | activate the highlighted/clicked button |
| `a` / `Ctrl+P` / `Ctrl+O` | jump straight to add / presets & stacks / import, same as their buttons |

### Navigation (once at least one service exists)

| Key | Action |
|---|---|
| `←` / `→` | switch focus between the three panes |
| `↑` / `↓`, mouse wheel | services list: move selection · YAML preview: scroll |
| `a` | add a new service (prompts for a name) |
| `d` | delete the selected service (prompts for confirmation) |
| `Enter` / `e` | edit the selected service (center pane must be focused) |

### Editing a service

| Key | Action |
|---|---|
| `↑` / `↓` | move within a multi-line field (ports/environment/volumes); at the top/bottom edge, switches to the previous/next field |
| `Tab` / `Shift+Tab` | switch field directly |
| `Enter` | insert a new line, in ports/environment/volumes |
| `Ctrl+S` | save the service and return to the normal view |
| `Esc` | discard changes and return — asks for confirmation only if anything actually changed |

### Presets, stacks, validation, import

| Key | Action |
|---|---|
| `Ctrl+P` | open the preset/stack picker — `←`/`→` switches between the **Presets** and **Stacks** tabs, `↑`/`↓` navigates, `Enter` adds the selection |
| `Ctrl+V` | run validation — shows errors and warnings for the current config, scrollable with `↑`/`↓` or the mouse wheel |
| `Ctrl+O` | import an existing `docker-compose.yml` |

### Saving and quitting

| Key | Action |
|---|---|
| `Ctrl+S` | save — prompts for a path (default `docker-compose.yml`), asks before overwriting an existing file |
| `q` | quit — if there are unsaved changes, opens the same save prompt with an explicit "quit without saving" option; quits immediately otherwise |
| `Ctrl+C` | quit immediately, no prompt |

## Project layout

- `pkg/composer` — the domain model: services, presets, stacks, YAML
  import/export, and validation. No TUI dependencies — usable as a
  library on its own.
- `internal/tui` — the Bubble Tea/Lipgloss terminal UI.
