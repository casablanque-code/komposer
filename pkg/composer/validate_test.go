package composer

import (
	"strings"
	"testing"
)

func hasError(r ValidationResult, field string) bool {
	for _, e := range r.Errors {
		if e.Field == field {
			return true
		}
	}
	return false
}

func hasWarning(r ValidationResult, field string) bool {
	for _, w := range r.Warnings {
		if w.Field == field {
			return true
		}
	}
	return false
}

func TestValidateEmptyConfig(t *testing.T) {
	c := NewComposeConfig()
	r := c.Validate()
	if r.IsValid() {
		t.Fatalf("a config with no services should be invalid")
	}
	if !hasError(r, "services") {
		t.Fatalf("expected a 'services' error, got %+v", r.Errors)
	}
}

func TestValidateDuplicateServiceName(t *testing.T) {
	c := NewComposeConfig()
	c.Services = append(c.Services,
		ServiceEntry{Name: "web", Config: &ServiceConfig{Image: "nginx"}},
		ServiceEntry{Name: "web", Config: &ServiceConfig{Image: "nginx"}},
	)
	r := c.Validate()
	if !hasError(r, "name") {
		t.Fatalf("expected a duplicate-name error, got %+v", r.Errors)
	}
}

func TestValidateRequiresImageOrBuild(t *testing.T) {
	c := NewComposeConfig()
	c.AddService("web")
	r := c.Validate()
	if !hasError(r, "image/build") {
		t.Fatalf("expected an image/build error for a service with neither, got %+v", r.Errors)
	}
}

func TestValidateRejectsBothImageAndBuild(t *testing.T) {
	c := NewComposeConfig()
	svc := c.AddService("web")
	svc.Image = "nginx"
	svc.Build = "."
	r := c.Validate()
	if !hasError(r, "image/build") {
		t.Fatalf("expected an image/build error when both are set, got %+v", r.Errors)
	}
}

func TestValidateValidConfigHasNoErrors(t *testing.T) {
	c := NewComposeConfig()
	svc := c.AddService("web")
	svc.Image = "nginx"
	svc.Ports = []string{"127.0.0.1:8080:80"}
	svc.Restart = "unless-stopped"
	r := c.Validate()
	if !r.IsValid() {
		t.Fatalf("expected a valid config, got errors: %+v", r.Errors)
	}
}

func TestValidatePortFormats(t *testing.T) {
	cases := []struct {
		port  string
		valid bool
	}{
		{"80", true},
		{"8080:80", true},
		{"127.0.0.1:8080:80", true},
		{"127.0.0.1::8080", true},
		{"8080/tcp", true},
		{"8080:80/udp", true},
		{"", false},
		{"not-a-port", false},
		{"99999", false},
		{"0", false},
		{"1:2:3:4", false},
	}
	for _, tc := range cases {
		if got := isValidPort(tc.port); got != tc.valid {
			t.Errorf("isValidPort(%q) = %v, want %v", tc.port, got, tc.valid)
		}
	}
}

func TestValidateInvalidPortProducesError(t *testing.T) {
	c := NewComposeConfig()
	svc := c.AddService("web")
	svc.Image = "nginx"
	svc.Ports = []string{"not-a-port"}
	r := c.Validate()
	if !hasError(r, "ports") {
		t.Fatalf("expected a ports error, got %+v", r.Errors)
	}
}

func TestValidateEnvironmentFormat(t *testing.T) {
	c := NewComposeConfig()
	svc := c.AddService("web")
	svc.Image = "nginx"
	svc.Environment = []string{"NO_EQUALS_SIGN"}
	r := c.Validate()
	if !hasError(r, "environment") {
		t.Fatalf("expected an environment format error, got %+v", r.Errors)
	}
}

func TestValidateRestartPolicy(t *testing.T) {
	c := NewComposeConfig()
	svc := c.AddService("web")
	svc.Image = "nginx"
	svc.Restart = "sometimes"
	r := c.Validate()
	if !hasError(r, "restart") {
		t.Fatalf("expected a restart policy error, got %+v", r.Errors)
	}
}

func TestValidateHealthcheckRequiresTest(t *testing.T) {
	c := NewComposeConfig()
	svc := c.AddService("web")
	svc.Image = "nginx"
	svc.HealthCheck = &HealthCheck{Interval: "30s"}
	r := c.Validate()
	if !hasError(r, "healthcheck.test") {
		t.Fatalf("expected a healthcheck.test error, got %+v", r.Errors)
	}
}

func TestValidateHealthcheckInvalidDuration(t *testing.T) {
	c := NewComposeConfig()
	svc := c.AddService("web")
	svc.Image = "nginx"
	svc.HealthCheck = &HealthCheck{Test: []string{"CMD", "true"}, Interval: "banana"}
	r := c.Validate()
	if !hasError(r, "healthcheck.interval") {
		t.Fatalf("expected a healthcheck.interval error, got %+v", r.Errors)
	}
}

func TestValidateDependsOnMissingReference(t *testing.T) {
	c := NewComposeConfig()
	svc := c.AddService("web")
	svc.Image = "nginx"
	svc.DependsOn = []DependsOnEntry{{Service: "ghost", Condition: CondServiceStarted}}
	r := c.Validate()
	if !hasError(r, "depends_on") {
		t.Fatalf("expected a depends_on error for a nonexistent service, got %+v", r.Errors)
	}
}

func TestValidateWarnsOnPortExposedToAllInterfaces(t *testing.T) {
	c := NewComposeConfig()
	svc := c.AddService("web")
	svc.Image = "nginx"
	svc.Ports = []string{"8080:80"}
	r := c.Validate()
	if !hasWarning(r, "ports") {
		t.Fatalf("expected a ports warning for a port with no host IP, got %+v", r.Warnings)
	}
}

func TestValidateNoWarningForLoopbackPort(t *testing.T) {
	c := NewComposeConfig()
	svc := c.AddService("web")
	svc.Image = "nginx"
	svc.Ports = []string{"127.0.0.1:8080:80"}
	r := c.Validate()
	if hasWarning(r, "ports") {
		t.Fatalf("did not expect a ports warning for a loopback-bound port, got %+v", r.Warnings)
	}
}

func TestValidateWarnsOnPrivileged(t *testing.T) {
	c := NewComposeConfig()
	svc := c.AddService("web")
	svc.Image = "nginx"
	svc.Privileged = true
	r := c.Validate()
	if !hasWarning(r, "privileged") {
		t.Fatalf("expected a privileged warning, got %+v", r.Warnings)
	}
}

func TestValidateWarnsOnRootUser(t *testing.T) {
	c := NewComposeConfig()
	svc := c.AddService("web")
	svc.Image = "nginx"
	svc.User = "root"
	r := c.Validate()
	if !hasWarning(r, "user") {
		t.Fatalf("expected a user warning for root, got %+v", r.Warnings)
	}
}

func TestValidateWarnsOnHardcodedSecret(t *testing.T) {
	c := NewComposeConfig()
	svc := c.AddService("web")
	svc.Image = "nginx"
	svc.Environment = []string{"DB_PASSWORD=hunter2"}
	r := c.Validate()
	if !hasWarning(r, "environment") {
		t.Fatalf("expected an environment warning for a hardcoded secret, got %+v", r.Warnings)
	}
}

func TestValidateWarnsOnEmptySecret(t *testing.T) {
	c := NewComposeConfig()
	svc := c.AddService("web")
	svc.Image = "nginx"
	svc.Environment = []string{"API_TOKEN="}
	r := c.Validate()
	if !hasWarning(r, "environment") {
		t.Fatalf("expected an environment warning for an empty secret, got %+v", r.Warnings)
	}
}

func TestValidateNoWarningForSubstitutedSecret(t *testing.T) {
	c := NewComposeConfig()
	svc := c.AddService("web")
	svc.Image = "nginx"
	svc.Environment = []string{"DB_PASSWORD=${DB_PASSWORD}"}
	r := c.Validate()
	if hasWarning(r, "environment") {
		t.Fatalf("did not expect a warning for a ${...}-substituted secret, got %+v", r.Warnings)
	}
}

func TestValidateWarnsOnDatabaseWithNoVolume(t *testing.T) {
	c := NewComposeConfig()
	svc := c.AddService("db")
	svc.Image = "postgres:16"
	r := c.Validate()
	if !hasWarning(r, "volumes") {
		t.Fatalf("expected a volumes warning for a database image with no volumes, got %+v", r.Warnings)
	}
}

func TestValidateNoVolumeWarningWhenVolumePresent(t *testing.T) {
	c := NewComposeConfig()
	svc := c.AddService("db")
	svc.Image = "postgres:16"
	svc.Volumes = []string{"dbdata:/var/lib/postgresql/data"}
	r := c.Validate()
	if hasWarning(r, "volumes") {
		t.Fatalf("did not expect a volumes warning once a volume is configured, got %+v", r.Warnings)
	}
}

func TestValidateWarnsOnExposedPostgresWithoutPassword(t *testing.T) {
	c := NewComposeConfig()
	svc := c.AddService("db")
	svc.Image = "postgres:16"
	svc.Ports = []string{"5432:5432"}
	svc.Volumes = []string{"dbdata:/var/lib/postgresql/data"}
	r := c.Validate()
	if !hasWarning(r, "environment") {
		t.Fatalf("expected an environment warning for exposed postgres with no password, got %+v", r.Warnings)
	}
}

// TestValidateNoWarningForPostgresWithPassword checks specifically for
// the absence of the "no POSTGRES_PASSWORD set" warning once a
// password is configured — a hardcoded-secret warning for
// POSTGRES_PASSWORD's own value is a separate, still-expected warning
// and isn't what this test is about.
func TestValidateNoWarningForPostgresWithPassword(t *testing.T) {
	c := NewComposeConfig()
	svc := c.AddService("db")
	svc.Image = "postgres:16"
	svc.Ports = []string{"5432:5432"}
	svc.Volumes = []string{"dbdata:/var/lib/postgresql/data"}
	svc.Environment = []string{"POSTGRES_PASSWORD=hunter2"}
	r := c.Validate()
	for _, w := range r.Warnings {
		if strings.Contains(w.Message, "no POSTGRES_PASSWORD set") {
			t.Fatalf("did not expect the 'no POSTGRES_PASSWORD set' warning once one is set, got %+v", r.Warnings)
		}
	}
}

func TestValidatePortExposedToAllInterfaces(t *testing.T) {
	cases := []struct {
		port     string
		exposed  bool
		testName string
	}{
		{"8080:80", true, "no host ip"},
		{"80", true, "container port only"},
		{"0.0.0.0:8080:80", true, "explicit 0.0.0.0"},
		{"127.0.0.1:8080:80", false, "explicit loopback"},
	}
	for _, tc := range cases {
		if got := isPortExposedToAllInterfaces(tc.port); got != tc.exposed {
			t.Errorf("isPortExposedToAllInterfaces(%q) [%s] = %v, want %v", tc.port, tc.testName, got, tc.exposed)
		}
	}
}

func TestSuggestLocalhostPort(t *testing.T) {
	cases := []struct{ in, want string }{
		{"8080", "127.0.0.1::8080"},
		{"8080:80", "127.0.0.1:8080:80"},
		{"0.0.0.0:8080:80", "127.0.0.1:8080:80"},
		{"8080:80/udp", "127.0.0.1:8080:80/udp"},
	}
	for _, tc := range cases {
		if got := suggestLocalhostPort(tc.in); got != tc.want {
			t.Errorf("suggestLocalhostPort(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
