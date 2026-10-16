package composer

import (
	"fmt"
	"path"
	"strings"

	"gopkg.in/yaml.v3"
)

// This file holds the security and reliability checks added on top of the
// original set in validate.go. Everything here reads a service through
// both its modeled fields and ServiceConfig.Extra (see service_views.go),
// so a service is judged by what it actually says, not by which of the
// two places the importer happened to put it in.

// validateSecurity runs the checks in this file against one service.
func validateSecurity(name string, cfg *ServiceConfig, result *ValidationResult) {
	validateSensitiveMounts(name, cfg, result)
	validateHostNamespaces(name, cfg, result)
	validateNoNewPrivileges(name, cfg, result)
	validateDatabaseAuth(name, cfg, result)
}

// ---- image tags -----------------------------------------------------

// rollingTags are tag names that conventionally move to a new release
// over time, like "latest" does.
var rollingTags = map[string]bool{
	"latest": true, "stable": true, "edge": true, "main": true, "master": true,
	"nightly": true, "dev": true, "develop": true, "rolling": true, "testing": true,
	"canary": true, "beta": true, "alpha": true, "unstable": true, "current": true, "next": true,
}

// isRollingTag reports whether tag is one of rollingTags, or such a name
// with a variant suffix that carries no version ("stable-alpine",
// "main-slim"). A suffix with digits ("nightly-20241001") pins a build and
// is not flagged.
func isRollingTag(tag string) bool {
	t := strings.ToLower(tag)
	if rollingTags[t] {
		return true
	}
	first, rest, found := strings.Cut(t, "-")
	return found && rollingTags[first] && !strings.ContainsAny(rest, "0123456789")
}

// ---- mounts ---------------------------------------------------------

// sensitiveHostDirs are host directories that are dangerous to hand to a
// container with write access: modifying them can change how the host
// boots, authenticates, or runs its own containers. Matched exactly (after
// cleaning) rather than by prefix, because mounting a single file or
// subdirectory such as /etc/localtime or /etc/ssl/certs read-only is routine.
var sensitiveHostDirs = map[string]bool{
	"/":               true,
	"/boot":           true,
	"/dev":            true,
	"/etc":            true,
	"/home":           true,
	"/lib":            true,
	"/proc":           true,
	"/root":           true,
	"/run":            true,
	"/sys":            true,
	"/usr":            true,
	"/var/lib":        true,
	"/var/lib/docker": true,
	"/var/run":        true,
}

func validateSensitiveMounts(name string, cfg *ServiceConfig, result *ValidationResult) {
	for _, m := range cfg.Mounts() {
		if m.ReadOnly || !strings.HasPrefix(m.Source, "/") {
			continue
		}
		src := path.Clean(m.Source)
		if !sensitiveHostDirs[src] {
			continue
		}
		result.AddWarning(name, "volumes", fmt.Sprintf(
			"bind-mounts the host directory '%s' read-write — a compromised container could modify "+
				"the host itself through it; mount it read-only (':ro') or narrow the mount to just "+
				"the files the service needs", src))
	}
}

// ---- host namespaces ------------------------------------------------

func extraIsHost(cfg *ServiceConfig, key string) bool {
	node, ok := cfg.Extra[key]
	return ok && node.Kind == yaml.ScalarNode && strings.EqualFold(node.Value, "host")
}

func validateHostNamespaces(name string, cfg *ServiceConfig, result *ValidationResult) {
	if extraIsHost(cfg, "ipc") {
		result.AddWarning(name, "ipc",
			"runs with 'ipc: host' — the container shares the host's shared memory and message queues, "+
				"so it can read and interfere with other processes' IPC")
	}
	if extraIsHost(cfg, "userns_mode") {
		result.AddWarning(name, "userns_mode",
			"runs with 'userns_mode: host' — this turns off user-namespace remapping, so root inside "+
				"the container is root on the host")
	}
	if extraIsHost(cfg, "cgroup") {
		result.AddWarning(name, "cgroup",
			"runs with 'cgroup: host' — the container shares the host's cgroup namespace and can see "+
				"every cgroup on the machine")
	}
}

// ---- no-new-privileges ----------------------------------------------

func validateNoNewPrivileges(name string, cfg *ServiceConfig, result *ValidationResult) {
	node, ok := cfg.Extra["security_opt"]
	if !ok {
		return
	}
	opts, ok := decodeScalarList(&node)
	if !ok {
		return
	}
	for _, opt := range opts {
		norm := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(opt), " ", ""))
		if norm == "no-new-privileges:false" || norm == "no-new-privileges=false" {
			result.AddWarning(name, "security_opt", fmt.Sprintf(
				"'%s' explicitly lets processes in the container gain more privileges through "+
					"setuid binaries; remove it, or set 'no-new-privileges:true'", opt))
		}
	}
}

// ---- databases without authentication -------------------------------

// imageBase returns the lower-cased image name without registry, tag or
// digest: "ghcr.io/acme/Postgres:16" -> "postgres".
func imageBase(image string) string {
	name := image
	if i := strings.Index(name, "@"); i >= 0 {
		name = name[:i]
	}
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	if i := strings.Index(name, ":"); i >= 0 {
		name = name[:i]
	}
	return strings.ToLower(name)
}

// envLookup finds key (case-insensitive) in an environment list. bare
// is true for a "KEY" entry with no "=", which passes the value through
// from the host: it counts as set, but its value is unknown ("").
func envLookup(env []string, key string) (value string, found, bare bool) {
	for _, entry := range env {
		k, v, hasValue := strings.Cut(entry, "=")
		if !strings.EqualFold(strings.TrimSpace(k), key) {
			continue
		}
		return strings.TrimSpace(v), true, !hasValue
	}
	return "", false, false
}

// envProvided reports whether any of keys (or its "_FILE" variant, the
// Docker secrets convention) is set to a non-empty value or passed
// through from the host.
func envProvided(env []string, keys ...string) bool {
	for _, key := range keys {
		for _, k := range []string{key, key + "_FILE"} {
			if v, found, bare := envLookup(env, k); found && (bare || v != "") {
				return true
			}
		}
	}
	return false
}

func envTruthy(env []string, key string) (value string, truthy bool) {
	v, found, _ := envLookup(env, key)
	if !found {
		return "", false
	}
	switch strings.ToLower(v) {
	case "1", "true", "yes", "y", "on":
		return v, true
	}
	return v, false
}

// extraText returns the scalar text of the given Extra keys (command,
// entrypoint: either a string or a list), joined with spaces.
func extraText(cfg *ServiceConfig, keys ...string) string {
	var parts []string
	for _, key := range keys {
		node, ok := cfg.Extra[key]
		if !ok {
			continue
		}
		switch node.Kind {
		case yaml.ScalarNode:
			parts = append(parts, node.Value)
		case yaml.SequenceNode:
			for _, item := range node.Content {
				if item.Kind == yaml.ScalarNode {
					parts = append(parts, item.Value)
				}
			}
		}
	}
	return strings.Join(parts, " ")
}

var (
	postgresImages = map[string]bool{"postgres": true, "postgresql": true, "timescaledb": true, "postgis": true, "pgvector": true}
	mongoImages    = map[string]bool{"mongo": true, "mongodb": true}
	mysqlImages    = map[string]bool{"mysql": true, "mariadb": true}
)

func hasExposedPort(cfg *ServiceConfig) bool {
	for _, port := range cfg.PortStrings() {
		if isValidPort(port) && isPortExposedToAllInterfaces(port) {
			return true
		}
	}
	return false
}

// validateDatabaseAuth flags well-known database images that are
// published on the network (or configured outright) with authentication
// off. Images are matched by name, not substring, so a metrics exporter
// like "postgres-exporter" isn't mistaken for the database itself. When
// the service has an env_file the real environment is unknowable from
// here, so the "no password set" checks stay quiet rather than guess.
func validateDatabaseAuth(name string, cfg *ServiceConfig, result *ValidationResult) {
	if cfg.Image == "" {
		return
	}
	base := imageBase(cfg.Image)
	envUnknown := cfg.hasExtra("env_file")
	exposed := hasExposedPort(cfg)

	switch {
	case postgresImages[base]:
		if v, found, _ := envLookup(cfg.Environment, "POSTGRES_HOST_AUTH_METHOD"); found && strings.EqualFold(v, "trust") {
			msg := "'POSTGRES_HOST_AUTH_METHOD=trust' lets every client that can reach this database in " +
				"without a password"
			if exposed {
				msg += " — and its port is published on all interfaces"
			}
			result.AddWarning(name, "environment", msg)
		}
		if exposed && !envUnknown && !envProvided(cfg.Environment, "POSTGRES_PASSWORD", "POSTGRESQL_PASSWORD") {
			result.AddWarning(name, "environment",
				"postgres is published on the network with no POSTGRES_PASSWORD set — "+
					"without one, anyone who can reach this port has full database access")
		}

	case mongoImages[base]:
		cmd := strings.ToLower(extraText(cfg, "command", "entrypoint"))
		authConfigured := envProvided(cfg.Environment, "MONGO_INITDB_ROOT_USERNAME", "MONGODB_ROOT_PASSWORD") ||
			strings.Contains(cmd, "--auth") || strings.Contains(cmd, "--keyfile")
		if exposed && !envUnknown && !authConfigured {
			result.AddWarning(name, "environment",
				"mongo is published on the network with no authentication configured (no "+
					"MONGO_INITDB_ROOT_USERNAME/PASSWORD, no --auth) — MongoDB then starts without access "+
					"control, so anyone who can reach this port has full database access")
		}

	case mysqlImages[base]:
		for _, key := range []string{"MYSQL_ALLOW_EMPTY_PASSWORD", "MARIADB_ALLOW_EMPTY_ROOT_PASSWORD"} {
			if v, truthy := envTruthy(cfg.Environment, key); truthy {
				msg := fmt.Sprintf("'%s=%s' lets the root user connect with an empty password; set a root password instead", key, v)
				if exposed {
					msg += " — and this service's port is published on all interfaces"
				}
				result.AddWarning(name, "environment", msg)
			}
		}
	}
}

// ---- depends_on cycles ----------------------------------------------

// validateDependencyCycles reports every distinct depends_on cycle once.
// Compose refuses to start a stack that has one, so this is an Error.
func (c *ComposeConfig) validateDependencyCycles(result *ValidationResult) {
	deps := make(map[string][]string)
	var order []string
	for _, entry := range c.Services {
		order = append(order, entry.Name)
		for _, d := range entry.Config.DependsOn {
			deps[entry.Name] = append(deps[entry.Name], d.Service)
		}
	}

	const (
		unvisited = iota
		inProgress
		done
	)
	state := make(map[string]int)
	var stack []string
	reported := make(map[string]bool)

	var visit func(string)
	visit = func(n string) {
		state[n] = inProgress
		stack = append(stack, n)
		for _, next := range deps[n] {
			if !c.hasService(next) {
				// A reference to a service that doesn't exist is
				// reported separately.
				continue
			}
			switch state[next] {
			case unvisited:
				visit(next)
			case inProgress:
				start := 0
				for i, s := range stack {
					if s == next {
						start = i
						break
					}
				}
				cycle := append([]string(nil), stack[start:]...)
				key := canonicalCycle(cycle)
				if !reported[key] {
					reported[key] = true
					result.Add(cycle[0], "depends_on", fmt.Sprintf(
						"dependency cycle: %s — Compose will refuse to start this stack",
						strings.Join(append(cycle, cycle[0]), " → ")))
				}
			}
		}
		stack = stack[:len(stack)-1]
		state[n] = done
	}

	for _, n := range order {
		if state[n] == unvisited {
			visit(n)
		}
	}
}

func (c *ComposeConfig) hasService(name string) bool {
	return c.GetService(name) != nil
}

// canonicalCycle rotates a cycle so its alphabetically smallest member
// comes first, giving the same key however the cycle was entered.
func canonicalCycle(cycle []string) string {
	min := 0
	for i, s := range cycle {
		if s < cycle[min] {
			min = i
		}
	}
	rotated := append(append([]string(nil), cycle[min:]...), cycle[:min]...)
	return strings.Join(rotated, "\x00")
}
