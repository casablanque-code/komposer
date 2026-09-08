package composer

import (
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// ValidationError represents a single validation issue. It's used for
// both hard errors (invalid syntax, things Docker would reject) and
// advisory warnings (valid syntax with a risky or surprising default) —
// see ValidationResult.Warnings.
type ValidationError struct {
	Service string // empty for config-level errors
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	if e.Service != "" {
		return fmt.Sprintf("service '%s': %s: %s", e.Service, e.Field, e.Message)
	}
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// ValidationResult holds everything found while validating a config.
// Errors are things that are actually wrong (bad syntax, a reference to
// a service that doesn't exist) — IsValid() is based on these alone.
// Warnings are valid, syntactically-correct configuration that has a
// commonly-surprising default or a well-known security footgun (e.g. a
// port published on every interface, a hardcoded secret). Warnings
// never block saving or exporting; they're advisory only.
type ValidationResult struct {
	Errors   []ValidationError
	Warnings []ValidationError
}

// IsValid returns true if there are no validation errors. Warnings do
// not affect this — they're informational, not blocking.
func (r ValidationResult) IsValid() bool {
	return len(r.Errors) == 0
}

// Add appends a validation error to the result.
func (r *ValidationResult) Add(service, field, message string) {
	r.Errors = append(r.Errors, ValidationError{
		Service: service,
		Field:   field,
		Message: message,
	})
}

// AddWarning appends a non-blocking advisory warning to the result.
func (r *ValidationResult) AddWarning(service, field, message string) {
	r.Warnings = append(r.Warnings, ValidationError{
		Service: service,
		Field:   field,
		Message: message,
	})
}

var portPattern = regexp.MustCompile(`^(\d+:)?\d+(/tcp|/udp)?$`)

// secretEnvKeyPattern matches environment variable names that
// conventionally hold a sensitive value.
var secretEnvKeyPattern = regexp.MustCompile(`(?i)(password|secret|token|api[_-]?key|private[_-]?key|access[_-]?key)`)

// dangerousCapabilities are Linux capabilities that, added via
// cap_add, meaningfully widen a container's access to the host beyond
// Docker's already-reduced default set — roughly comparable in effect
// to 'privileged: true' for the specific things they each unlock
// (arbitrary device/kernel access, host networking config, tracing
// other processes, loading kernel modules). Not every capability is
// flagged: most of Docker's default-dropped set (e.g. NET_RAW,
// SYS_CHROOT) is routine for images that need it and not worth
// warning about.
var dangerousCapabilities = map[string]bool{
	"ALL":        true,
	"SYS_ADMIN":  true,
	"NET_ADMIN":  true,
	"SYS_PTRACE": true,
	"SYS_MODULE": true,
}

// Validate checks the entire ComposeConfig for common errors.
func (c *ComposeConfig) Validate() ValidationResult {
	result := ValidationResult{}

	if len(c.Services) == 0 {
		result.Add("", "services", "at least one service is required")
		return result
	}

	serviceNames := make(map[string]bool)
	for _, entry := range c.Services {
		// Check for duplicate service names
		if serviceNames[entry.Name] {
			result.Add(entry.Name, "name", "duplicate service name")
			continue
		}
		serviceNames[entry.Name] = true

		// Validate individual service
		c.validateService(entry.Name, entry.Config, &result)
	}

	// Check depends_on references
	for _, entry := range c.Services {
		for _, dep := range entry.Config.DependsOn {
			if !serviceNames[dep.Service] {
				result.Add(entry.Name, "depends_on", fmt.Sprintf("references non-existent service '%s'", dep.Service))
				continue
			}
			// condition: service_healthy is a promise, not just a
			// preference — Compose itself refuses to start a stack
			// where the referenced service has no healthcheck defined
			// to actually satisfy that condition, so this is an Error
			// (would break `docker compose up`), not a Warning.
			if dep.Condition == CondServiceHealthy {
				if target := c.GetService(dep.Service); target != nil && target.HealthCheck == nil {
					result.Add(entry.Name, "depends_on", fmt.Sprintf(
						"depends on '%s' with condition 'service_healthy', but '%s' has no healthcheck configured — "+
							"Compose will refuse to start this stack; either add a healthcheck to '%s' or "+
							"change the condition to 'service_started'",
						dep.Service, dep.Service, dep.Service))
				}
			}
		}
	}

	return result
}

func (c *ComposeConfig) validateService(name string, cfg *ServiceConfig, result *ValidationResult) {
	// Either image or build must be specified
	if cfg.Image == "" && cfg.Build == "" {
		result.Add(name, "image/build", "either 'image' or 'build' must be specified")
	}

	// Both image and build shouldn't be set simultaneously
	if cfg.Image != "" && cfg.Build != "" {
		result.Add(name, "image/build", "cannot specify both 'image' and 'build'")
	}

	// An image with no tag at all, or an explicit ':latest', both
	// resolve to whatever 'latest' happens to point to right now —
	// Docker Compose doesn't pin anything for you. That's a common
	// footgun: the same compose file can quietly pull a different
	// image on every `docker compose pull`/`up`, with no diff in the
	// file to explain why something changed. A digest reference
	// (image@sha256:...) is the one case that's already fully pinned,
	// so it's excluded rather than flagged.
	if cfg.Image != "" {
		if tag, pinned := parseImageTag(cfg.Image); !pinned {
			switch tag {
			case "":
				result.AddWarning(name, "image", fmt.Sprintf(
					"'%s' has no tag — Docker defaults to ':latest', which can pull a different image on every 'docker compose pull' with no change to this file; pin an explicit version",
					cfg.Image))
			case "latest":
				result.AddWarning(name, "image", fmt.Sprintf(
					"'%s' is explicitly pinned to ':latest', which can still pull a different image on every 'docker compose pull' with no change to this file; pin an explicit version instead",
					cfg.Image))
			}
		}
	}

	// Validate port formats
	for _, port := range cfg.Ports {
		if !isValidPort(port) {
			result.Add(name, "ports", fmt.Sprintf("invalid port format: '%s' (expected: [ip:][host:]container[/protocol])", port))
		}
	}

	// Validate environment variables format
	for _, env := range cfg.Environment {
		if !strings.Contains(env, "=") {
			result.Add(name, "environment", fmt.Sprintf("invalid format: '%s' (expected: KEY=value)", env))
		}
	}

	// Validate restart policy
	if cfg.Restart != "" {
		validRestarts := map[string]bool{
			"no":             true,
			"always":         true,
			"on-failure":     true,
			"unless-stopped": true,
		}
		if !validRestarts[cfg.Restart] {
			result.Add(name, "restart", fmt.Sprintf("invalid policy: '%s' (expected: no, always, on-failure, unless-stopped)", cfg.Restart))
		}
	}

	// Validate healthcheck if present
	if cfg.HealthCheck != nil {
		if len(cfg.HealthCheck.Test) == 0 {
			result.Add(name, "healthcheck.test", "test command is required")
		}
		if cfg.HealthCheck.Interval != "" && !isValidDuration(cfg.HealthCheck.Interval) {
			result.Add(name, "healthcheck.interval", fmt.Sprintf("invalid duration: '%s'", cfg.HealthCheck.Interval))
		}
		if cfg.HealthCheck.Timeout != "" && !isValidDuration(cfg.HealthCheck.Timeout) {
			result.Add(name, "healthcheck.timeout", fmt.Sprintf("invalid duration: '%s'", cfg.HealthCheck.Timeout))
		}
	}

	// --- Advisory warnings below. These are all syntactically valid
	// docker-compose and Docker will happily run them — they're flagged
	// because each one is a well-known footgun with a safer alternative,
	// not because they're wrong. None of these block saving. ---

	// A published port with no explicit host IP (or an explicit
	// non-loopback IP) binds to every interface on the machine — that's
	// Docker's actual default behavior, not a mistake, but it's a
	// common surprise for anyone who assumed "8080:80" meant
	// "reachable from this machine only".
	for _, port := range cfg.Ports {
		if isValidPort(port) && isPortExposedToAllInterfaces(port) {
			result.AddWarning(name, "ports", fmt.Sprintf(
				"'%s' is published on all network interfaces (Docker's default) — "+
					"if you only need it reachable from this machine, use '%s' instead",
				port, suggestLocalhostPort(port)))
		}
	}

	if cfg.Privileged {
		result.AddWarning(name, "privileged", "runs with 'privileged: true' — this gives the container "+
			"full access to the host's devices and kernel capabilities; only use it if the service genuinely needs it")
	}

	if cfg.User == "root" || cfg.User == "0" {
		result.AddWarning(name, "user", "explicitly runs as 'root' — consider a non-root user if the image supports it")
	}

	// The remaining checks below all read from cfg.Extra rather than a
	// dedicated struct field, since none of cap_add/network_mode/pid/
	// security_opt have one yet (see ServiceConfig.Extra) — same
	// pattern import.go's decodeScalarList already uses for anything
	// modeled as a plain list of scalars.

	// A bind-mounted Docker socket is a well-known container-escape
	// vector: whoever can reach this container can talk to the host's
	// Docker daemon directly, which is equivalent to root on the host.
	// Legitimate uses exist (Portainer, Watchtower, CI runners), so
	// this is advisory, same as 'privileged' above.
	for _, vol := range cfg.Volumes {
		if strings.Contains(vol, "/var/run/docker.sock") {
			result.AddWarning(name, "volumes",
				"mounts the Docker socket ('/var/run/docker.sock') — anyone who can reach this "+
					"container can control the host's Docker daemon, which is equivalent to root on "+
					"the host; only do this if the service genuinely needs it (e.g. Portainer, Watchtower)")
			break
		}
	}

	if capNode, ok := cfg.Extra["cap_add"]; ok {
		if caps, ok := decodeScalarList(&capNode); ok {
			for _, capName := range caps {
				if dangerousCapabilities[strings.ToUpper(strings.TrimSpace(capName))] {
					result.AddWarning(name, "cap_add", fmt.Sprintf(
						"adds capability '%s' — this significantly widens what the container can do "+
							"to the host; only add capabilities the service specifically needs",
						capName))
				}
			}
		}
	}

	if modeNode, ok := cfg.Extra["network_mode"]; ok && modeNode.Kind == yaml.ScalarNode && strings.EqualFold(modeNode.Value, "host") {
		result.AddWarning(name, "network_mode",
			"runs with 'network_mode: host' — this removes network isolation between the container "+
				"and the host; only use it if the service genuinely needs it")
	}

	if pidNode, ok := cfg.Extra["pid"]; ok && pidNode.Kind == yaml.ScalarNode && strings.EqualFold(pidNode.Value, "host") {
		result.AddWarning(name, "pid",
			"runs with 'pid: host' — the container can see and interact with every process on the host")
	}

	if secOptNode, ok := cfg.Extra["security_opt"]; ok {
		if opts, ok := decodeScalarList(&secOptNode); ok {
			for _, opt := range opts {
				if strings.Contains(strings.ToLower(opt), "unconfined") {
					result.AddWarning(name, "security_opt", fmt.Sprintf(
						"'%s' disables a security sandbox (seccomp/AppArmor) for this container", opt))
				}
			}
		}
	}

	for _, env := range cfg.Environment {
		key, empty, hardcoded := classifySecretEnv(env)
		switch {
		case empty:
			result.AddWarning(name, "environment", fmt.Sprintf(
				"'%s' looks like a secret but has no value set — an empty password is "+
					"just as risky as a hardcoded one (some images even disable auth entirely "+
					"when it's blank); set it via env_file or ${%s}, or remove the key if it's genuinely unused",
				key, key))
		case hardcoded:
			result.AddWarning(name, "environment", fmt.Sprintf(
				"'%s' looks like a secret with a value hardcoded directly in the compose file — "+
					"consider an env_file, Docker secrets, or ${%s} substituted from your shell/CI instead",
				key, key))
		}
	}

	// A database/stateful service with no volumes at all loses every
	// bit of its data the moment the container is removed or recreated
	// — which `docker compose up` does routinely. This only fires for
	// images recognized as stateful (see databaseDataDirs); it has no
	// way to know that about an arbitrary custom image.
	if dir, ok := suggestedDataDir(cfg.Image); ok && len(cfg.Volumes) == 0 {
		result.AddWarning(name, "volumes", fmt.Sprintf(
			"'%s' looks like a database with no volumes configured — data will be lost "+
				"every time the container is removed or recreated; consider a volume such as './data:%s'",
			cfg.Image, dir))
	}

	// A database/stateful image with no healthcheck means depends_on
	// (this service's own, or another service's on it) can only ever
	// use condition: service_started — which fires the moment the
	// process starts listening, not once it's actually ready to
	// accept connections. Same recognized-image list as the no-volume
	// check above; same limitation, too — it has no way to know this
	// about an arbitrary custom image.
	if hc, ok := suggestedHealthCheck(cfg.Image); ok && cfg.HealthCheck == nil {
		result.AddWarning(name, "healthcheck", fmt.Sprintf(
			"'%s' looks like a database with no healthcheck configured — anything with "+
				"'depends_on: %s' can only wait for the process to start, not for it to actually "+
				"be ready to accept connections; consider a healthcheck such as %s",
			cfg.Image, name, formatHealthCheckTest(hc.Test)))
	}

	// The single most common "wide open database" mistake: Postgres
	// published on the network with no password configured at all —
	// whether POSTGRES_PASSWORD was never set or was set to an empty
	// value, either one leaves it reachable with no auth from anywhere
	// that can route to the port. This is deliberately specific to
	// Postgres for now (it's the concrete case that prompted it); the
	// same pattern could be extended to MySQL/Mongo/Redis's own
	// password env vars if that turns out to be worth it.
	if strings.Contains(strings.ToLower(cfg.Image), "postgres") {
		exposed := false
		for _, port := range cfg.Ports {
			if isValidPort(port) && isPortExposedToAllInterfaces(port) {
				exposed = true
				break
			}
		}
		if exposed && !hasNonEmptyEnv(cfg.Environment, "POSTGRES_PASSWORD") {
			result.AddWarning(name, "environment",
				"postgres is published on the network with no POSTGRES_PASSWORD set — "+
					"without one, anyone who can reach this port has full database access")
		}
	}
}

// isValidPort checks if a port mapping follows docker-compose format:
//   - "8080"                (container port only)
//   - "8080:80"              (host:container)
//   - "127.0.0.1:8080:80"    (ip:host:container)
//   - "8080/tcp"             (with protocol, any of the above forms)
func isValidPort(port string) bool {
	port = strings.TrimSpace(port)
	if port == "" {
		return false
	}

	p := strings.TrimSuffix(strings.TrimSuffix(port, "/tcp"), "/udp")

	parts := strings.Split(p, ":")
	switch len(parts) {
	case 1:
		return isValidPortNumber(parts[0])
	case 2:
		return isValidPortNumber(parts[0]) && isValidPortNumber(parts[1])
	case 3:
		if net.ParseIP(parts[0]) == nil {
			return false
		}
		// The host-port segment of "ip:host:container" may be empty,
		// meaning "pick a random host port" — e.g. "127.0.0.1::8080".
		if parts[1] != "" && !isValidPortNumber(parts[1]) {
			return false
		}
		return isValidPortNumber(parts[2])
	default:
		return false
	}
}

func isValidPortNumber(s string) bool {
	n, err := strconv.Atoi(s)
	return err == nil && n >= 1 && n <= 65535
}

// isPortExposedToAllInterfaces reports whether a ports entry (in any of
// the valid docker-compose short-syntax forms accepted by isValidPort)
// binds to every network interface on the host, rather than being
// restricted to loopback via an explicit "127.0.0.1:..." prefix.
func isPortExposedToAllInterfaces(port string) bool {
	p := strings.TrimSuffix(strings.TrimSuffix(strings.TrimSpace(port), "/tcp"), "/udp")
	parts := strings.Split(p, ":")
	if len(parts) < 3 {
		// "8080" or "8080:80" — no host IP given at all, so Docker
		// binds 0.0.0.0.
		return true
	}
	ip := net.ParseIP(parts[0])
	return !ip.IsLoopback()
}

// suggestLocalhostPort rewrites a ports entry to bind to 127.0.0.1
// instead of all interfaces, preserving whichever short-syntax form it
// was already using.
func suggestLocalhostPort(port string) string {
	proto := ""
	p := port
	for _, suf := range []string{"/tcp", "/udp"} {
		if strings.HasSuffix(p, suf) {
			proto = suf
			p = strings.TrimSuffix(p, suf)
			break
		}
	}

	parts := strings.Split(p, ":")
	switch len(parts) {
	case 1: // "8080" -> "127.0.0.1::8080" (random host port, loopback only)
		return "127.0.0.1::" + parts[0] + proto
	case 2: // "8080:80" -> "127.0.0.1:8080:80"
		return "127.0.0.1:" + parts[0] + ":" + parts[1] + proto
	case 3: // "0.0.0.0:8080:80" -> "127.0.0.1:8080:80"
		return "127.0.0.1:" + parts[1] + ":" + parts[2] + proto
	default:
		return "127.0.0.1:" + p + proto
	}
}

// classifySecretEnv looks at an "environment:" entry and, if its key
// looks like it's meant to hold a secret (by variable name convention),
// reports whether the value is empty or hardcoded. An entry using
// "${VAR}" substitution is considered fine either way — that's the
// recommended pattern, not a footgun — so it reports neither.
func classifySecretEnv(entry string) (key string, empty bool, hardcoded bool) {
	idx := strings.Index(entry, "=")
	if idx < 0 {
		return "", false, false
	}
	key = strings.TrimSpace(entry[:idx])
	if !secretEnvKeyPattern.MatchString(key) {
		return "", false, false
	}
	value := strings.TrimSpace(entry[idx+1:])
	if strings.HasPrefix(value, "${") {
		return key, false, false
	}
	if value == "" {
		return key, true, false
	}
	return key, false, true
}

// hasNonEmptyEnv reports whether cfg's environment list sets the given
// key (case-insensitive, matching how most images read env vars) to a
// non-empty value. Used for checks that care about one specific,
// well-known variable rather than the generic secret-name pattern.
func hasNonEmptyEnv(env []string, key string) bool {
	for _, entry := range env {
		idx := strings.Index(entry, "=")
		if idx < 0 {
			continue
		}
		k := strings.TrimSpace(entry[:idx])
		if !strings.EqualFold(k, key) {
			continue
		}
		return strings.TrimSpace(entry[idx+1:]) != ""
	}
	return false
}

// databaseDataDirs maps a substring found in an image name to the
// directory that image conventionally stores its persistent data in —
// used to give a copy-pasteable volume suggestion instead of just
// saying "add a volume somewhere".
var databaseDataDirs = []struct {
	match string
	dir   string
}{
	{"postgres", "/var/lib/postgresql/data"},
	{"mysql", "/var/lib/mysql"},
	{"mariadb", "/var/lib/mysql"},
	{"mongo", "/data/db"},
	{"redis", "/data"},
	{"rabbitmq", "/var/lib/rabbitmq"},
	{"clickhouse", "/var/lib/clickhouse"},
	{"elasticsearch", "/usr/share/elasticsearch/data"},
	{"cassandra", "/var/lib/cassandra"},
	{"couchdb", "/opt/couchdb/data"},
}

// parseImageTag extracts the tag portion of a Docker image reference,
// the way Docker itself resolves it: only the segment after the LAST
// '/' is considered (so a registry-with-port prefix like
// "localhost:5000/myimage" isn't mistaken for a tag — that colon is
// part of the registry host, not the image name), and only a colon
// within that final segment counts. pinned is true for a digest
// reference (image@sha256:...), which needs no tag at all to be fully
// reproducible; tag is "" when there's no tag and no digest, which is
// exactly the case Docker itself defaults to ":latest" for.
func parseImageTag(image string) (tag string, pinned bool) {
	if strings.Contains(image, "@") {
		return "", true
	}
	name := image
	if idx := strings.LastIndex(image, "/"); idx >= 0 {
		name = image[idx+1:]
	}
	if idx := strings.LastIndex(name, ":"); idx >= 0 {
		return name[idx+1:], false
	}
	return "", false
}

// suggestedDataDir reports the conventional data directory for a known
// database/stateful image, if the image name matches one of them.
func suggestedDataDir(image string) (dir string, ok bool) {
	lower := strings.ToLower(image)
	for _, d := range databaseDataDirs {
		if strings.Contains(lower, d.match) {
			return d.dir, true
		}
	}
	return "", false
}

// databaseHealthChecks are the same suggested commands Presets/Stacks
// already bake into their own database services (see presets.go) —
// this table exists so validateService can suggest the same thing for
// a database image a user typed in by hand instead of picking a
// preset, or one that arrived via an imported file. Interval/Timeout/
// Retries mirror the presets' own values for the same images where
// one already exists; for the rest, the same postgres/mysql/redis
// figures are reused since there's no established komposer precedent
// to diverge from.
var databaseHealthChecks = []struct {
	match string
	check HealthCheck
}{
	{"postgres", HealthCheck{Test: []string{"CMD-SHELL", "pg_isready -U postgres"}, Interval: "10s", Timeout: "5s", Retries: 5}},
	{"mysql", HealthCheck{Test: []string{"CMD", "mysqladmin", "ping", "-h", "localhost"}, Interval: "10s", Timeout: "5s", Retries: 5}},
	{"mariadb", HealthCheck{Test: []string{"CMD", "mysqladmin", "ping", "-h", "localhost"}, Interval: "10s", Timeout: "5s", Retries: 5}},
	{"redis", HealthCheck{Test: []string{"CMD", "redis-cli", "ping"}, Interval: "5s", Timeout: "3s", Retries: 3}},
	{"mongo", HealthCheck{Test: []string{"CMD", "mongosh", "--eval", "db.adminCommand('ping')"}, Interval: "10s", Timeout: "5s", Retries: 5}},
	{"rabbitmq", HealthCheck{Test: []string{"CMD", "rabbitmq-diagnostics", "-q", "ping"}, Interval: "10s", Timeout: "5s", Retries: 5}},
	{"elasticsearch", HealthCheck{Test: []string{"CMD-SHELL", "curl -sf http://localhost:9200/_cluster/health || exit 1"}, Interval: "10s", Timeout: "5s", Retries: 5}},
}

// suggestedHealthCheck reports a reasonable healthcheck for a known
// database/stateful image, if the image name matches one of them. The
// returned HealthCheck is a fresh copy the caller can attach directly
// (or modify) without aliasing databaseHealthChecks' own Test slice.
func suggestedHealthCheck(image string) (hc HealthCheck, ok bool) {
	lower := strings.ToLower(image)
	for _, d := range databaseHealthChecks {
		if strings.Contains(lower, d.match) {
			hc := d.check
			hc.Test = append([]string(nil), d.check.Test...)
			return hc, true
		}
	}
	return HealthCheck{}, false
}

// formatHealthCheckTest renders a healthcheck's Test command the way
// it'd actually be written in YAML — ["CMD", "redis-cli", "ping"] —
// for inclusion in a warning message, rather than Go's default slice
// formatting ([CMD redis-cli ping], with no quoting or commas).
func formatHealthCheckTest(test []string) string {
	quoted := make([]string, len(test))
	for i, t := range test {
		quoted[i] = fmt.Sprintf("%q", t)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

// isValidDuration checks if a string is a valid docker duration (e.g., "30s", "1m30s", "2h").
func isValidDuration(dur string) bool {
	dur = strings.TrimSpace(dur)
	if dur == "" {
		return false
	}

	// Simple pattern for docker durations: digit(s) followed by unit
	pattern := regexp.MustCompile(`^(\d+h)?(\d+m)?(\d+s)?(\d+ms)?$`)
	return pattern.MatchString(dur) && len(dur) > 1
}
