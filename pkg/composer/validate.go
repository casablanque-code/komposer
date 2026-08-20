package composer

import (
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
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

	for _, env := range cfg.Environment {
		if key, ok := looksLikeHardcodedSecret(env); ok {
			result.AddWarning(name, "environment", fmt.Sprintf(
				"'%s' looks like a secret with a value hardcoded directly in the compose file — "+
					"consider an env_file, Docker secrets, or ${%s} substituted from your shell/CI instead",
				key, key))
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

// looksLikeHardcodedSecret reports whether an "environment:" entry both
// looks like it's meant to hold a secret (by variable name convention)
// and has a literal, non-empty value baked in — as opposed to being
// left for the runtime environment to fill in via "${VAR}" shell
// substitution, which is the recommended pattern and is not flagged.
func looksLikeHardcodedSecret(entry string) (key string, ok bool) {
	idx := strings.Index(entry, "=")
	if idx < 0 {
		return "", false
	}
	key = strings.TrimSpace(entry[:idx])
	value := strings.TrimSpace(entry[idx+1:])
	if value == "" || strings.HasPrefix(value, "${") {
		return "", false
	}
	if !secretEnvKeyPattern.MatchString(key) {
		return "", false
	}
	return key, true
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
