package composer

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ValidationError represents a single validation issue.
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

// ValidationResult holds all validation errors found.
type ValidationResult struct {
	Errors []ValidationError
}

// IsValid returns true if there are no validation errors.
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

var portPattern = regexp.MustCompile(`^(\d+:)?\d+(/tcp|/udp)?$`)

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
			result.Add(name, "ports", fmt.Sprintf("invalid port format: '%s' (expected: [host:]container[/protocol])", port))
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
}

// isValidPort checks if a port mapping follows docker-compose format:
// - "8080" (container port only)
// - "8080:80" (host:container)
// - "8080/tcp" (with protocol)
// - "8080:80/tcp" (full format)
func isValidPort(port string) bool {
	port = strings.TrimSpace(port)
	if port == "" {
		return false
	}

	parts := strings.Split(port, ":")
	if len(parts) > 2 {
		return false
	}

	for _, part := range parts {
		// Remove protocol suffix if present
		p := strings.TrimSuffix(strings.TrimSuffix(part, "/tcp"), "/udp")

		// Check if it's a valid port number
		portNum, err := strconv.Atoi(p)
		if err != nil || portNum < 1 || portNum > 65535 {
			return false
		}
	}

	return true
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
