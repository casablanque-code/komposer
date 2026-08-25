package composer

import (
	"os"
	"path/filepath"
	"testing"
)

// importString writes yamlContent to a temp file and imports it,
// failing the test immediately on any import error.
func importString(t *testing.T, yamlContent string) *ComposeConfig {
	t.Helper()
	path := filepath.Join(t.TempDir(), "docker-compose.yml")
	if err := os.WriteFile(path, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	cfg, err := ImportYAML(path)
	if err != nil {
		t.Fatalf("ImportYAML: %v", err)
	}
	return cfg
}

func TestImportEnvironmentListForm(t *testing.T) {
	cfg := importString(t, `
services:
  web:
    image: nginx
    environment:
      - FOO=bar
      - BAZ=qux
`)
	got := cfg.GetService("web").Environment
	want := []string{"FOO=bar", "BAZ=qux"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("Environment = %v, want %v", got, want)
	}
}

// TestImportEnvironmentMapForm covers the crash this tool used to hit
// on any file using the (equally valid, arguably more common)
// mapping form of `environment:` — see parseEnvironment's doc comment.
func TestImportEnvironmentMapForm(t *testing.T) {
	cfg := importString(t, `
services:
  web:
    image: nginx
    environment:
      FOO: bar
      BAZ: qux
      PASSTHROUGH:
`)
	env := cfg.GetService("web").Environment
	got := map[string]bool{}
	for _, e := range env {
		got[e] = true
	}
	for _, want := range []string{"FOO=bar", "BAZ=qux", "PASSTHROUGH"} {
		if !got[want] {
			t.Fatalf("Environment %v missing normalized entry %q", env, want)
		}
	}
}

// TestImportLongSyntaxPortsAndVolumes covers the crash this tool used
// to hit on the long (mapping-per-entry) syntax for ports/volumes —
// see parseService's doc comment. It has no form field, so it must
// round-trip via Extra instead of aborting the import.
func TestImportLongSyntaxPortsAndVolumes(t *testing.T) {
	cfg := importString(t, `
services:
  web:
    image: nginx
    ports:
      - target: 80
        published: 8080
        protocol: tcp
    volumes:
      - type: volume
        source: data
        target: /data
volumes:
  data: {}
`)
	svc := cfg.GetService("web")
	if len(svc.Ports) != 0 {
		t.Fatalf("long-syntax ports should not populate the short-form Ports field, got %v", svc.Ports)
	}
	if len(svc.Volumes) != 0 {
		t.Fatalf("long-syntax volumes should not populate the short-form Volumes field, got %v", svc.Volumes)
	}
	if _, ok := svc.Extra["ports"]; !ok {
		t.Fatalf("long-syntax ports should be preserved in Extra")
	}
	if _, ok := svc.Extra["volumes"]; !ok {
		t.Fatalf("long-syntax volumes should be preserved in Extra")
	}

	out, err := cfg.ExportYAML()
	if err != nil {
		t.Fatalf("ExportYAML: %v", err)
	}
	// Round-trip: re-importing the export should still carry the same
	// long-syntax data verbatim in Extra (lossless round-trip).
	reimportPath := filepath.Join(t.TempDir(), "out.yml")
	if err := os.WriteFile(reimportPath, out, 0o644); err != nil {
		t.Fatalf("writing round-trip fixture: %v", err)
	}
	cfg2, err := ImportYAML(reimportPath)
	if err != nil {
		t.Fatalf("re-importing exported file: %v", err)
	}
	if _, ok := cfg2.GetService("web").Extra["ports"]; !ok {
		t.Fatalf("long-syntax ports did not survive an export/re-import round-trip")
	}
}

// TestImportBuildMappingForm covers `build:` given as a mapping
// (context/dockerfile/...) rather than the short string form — no
// form field for it, so it must round-trip via Extra rather than
// aborting the import.
func TestImportBuildMappingForm(t *testing.T) {
	cfg := importString(t, `
services:
  web:
    build:
      context: .
      dockerfile: Dockerfile.dev
`)
	svc := cfg.GetService("web")
	if svc.Build != "" {
		t.Fatalf("mapping-form build should not populate the short-form Build field, got %q", svc.Build)
	}
	if _, ok := svc.Extra["build"]; !ok {
		t.Fatalf("mapping-form build should be preserved in Extra")
	}
}

// TestImportHealthcheckStringTest covers `healthcheck.test:` given as
// its (also valid) plain-string shorthand instead of a list — no form
// field for that shape, so the whole healthcheck block round-trips via
// Extra rather than aborting the import.
func TestImportHealthcheckStringTest(t *testing.T) {
	cfg := importString(t, `
services:
  web:
    image: nginx
    healthcheck:
      test: curl -f http://localhost
      interval: 30s
`)
	svc := cfg.GetService("web")
	if svc.HealthCheck != nil {
		t.Fatalf("string-form healthcheck.test should not populate HealthCheck, got %+v", svc.HealthCheck)
	}
	if _, ok := svc.Extra["healthcheck"]; !ok {
		t.Fatalf("string-form healthcheck should be preserved in Extra")
	}
}

func TestImportHealthcheckListTest(t *testing.T) {
	cfg := importString(t, `
services:
  web:
    image: nginx
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost"]
      interval: 30s
      retries: 3
`)
	svc := cfg.GetService("web")
	if svc.HealthCheck == nil {
		t.Fatalf("list-form healthcheck.test should populate HealthCheck")
	}
	if svc.HealthCheck.Interval != "30s" || svc.HealthCheck.Retries != 3 {
		t.Fatalf("HealthCheck = %+v, unexpected values", svc.HealthCheck)
	}
	want := []string{"CMD", "curl", "-f", "http://localhost"}
	if len(svc.HealthCheck.Test) != len(want) {
		t.Fatalf("HealthCheck.Test = %v, want %v", svc.HealthCheck.Test, want)
	}
}

func TestImportDependsOnShortForm(t *testing.T) {
	cfg := importString(t, `
services:
  db:
    image: postgres
  web:
    image: nginx
    depends_on:
      - db
`)
	deps := cfg.GetService("web").DependsOn
	if len(deps) != 1 || deps[0].Service != "db" || deps[0].Condition != CondServiceStarted {
		t.Fatalf("DependsOn = %+v, want a single 'db' entry defaulting to service_started", deps)
	}
}

func TestImportDependsOnLongForm(t *testing.T) {
	cfg := importString(t, `
services:
  db:
    image: postgres
  web:
    image: nginx
    depends_on:
      db:
        condition: service_healthy
`)
	deps := cfg.GetService("web").DependsOn
	if len(deps) != 1 || deps[0].Service != "db" || deps[0].Condition != CondServiceHealthy {
		t.Fatalf("DependsOn = %+v, want a single 'db' entry with service_healthy", deps)
	}
}

func TestImportPreservesUnknownTopLevelAndServiceKeys(t *testing.T) {
	cfg := importString(t, `
services:
  web:
    image: nginx
    command: ["nginx", "-g", "daemon off;"]
    container_name: my-web
x-custom:
  foo: bar
`)
	if _, ok := cfg.Extra["x-custom"]; !ok {
		t.Fatalf("unknown top-level key x-custom should be preserved in ComposeConfig.Extra")
	}
	svc := cfg.GetService("web")
	if _, ok := svc.Extra["command"]; !ok {
		t.Fatalf("unknown service key command should be preserved in ServiceConfig.Extra")
	}
	if _, ok := svc.Extra["container_name"]; !ok {
		t.Fatalf("unknown service key container_name should be preserved in ServiceConfig.Extra")
	}
}

func TestImportUserAndPrivileged(t *testing.T) {
	cfg := importString(t, `
services:
  web:
    image: nginx
    user: root
    privileged: true
`)
	svc := cfg.GetService("web")
	if svc.User != "root" {
		t.Fatalf("User = %q, want root", svc.User)
	}
	if !svc.Privileged {
		t.Fatalf("Privileged = false, want true")
	}
}

func TestImportEmptyFileReturnsEmptyConfig(t *testing.T) {
	cfg := importString(t, "")
	if len(cfg.Services) != 0 {
		t.Fatalf("expected no services for an empty file, got %d", len(cfg.Services))
	}
}

func TestImportRejectsNonMappingDocument(t *testing.T) {
	path := filepath.Join(t.TempDir(), "docker-compose.yml")
	if err := os.WriteFile(path, []byte("- just\n- a\n- list\n"), 0o644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	if _, err := ImportYAML(path); err == nil {
		t.Fatalf("expected an error importing a document whose root isn't a mapping")
	}
}

func TestImportMissingFile(t *testing.T) {
	if _, err := ImportYAML(filepath.Join(t.TempDir(), "does-not-exist.yml")); err == nil {
		t.Fatalf("expected an error importing a nonexistent file")
	}
}
