package composer

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// exportedService exports cfg, decodes the result into plain maps and
// returns the named service. Decoding into a map doubles as a check
// that the output has no duplicate keys: yaml.v3 rejects those.
func exportedService(t *testing.T, cfg *ComposeConfig, name string) (map[string]any, string) {
	t.Helper()
	b, err := cfg.ExportYAML()
	if err != nil {
		t.Fatalf("ExportYAML: %v", err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(b, &doc); err != nil {
		t.Fatalf("exported YAML is invalid (duplicate key?): %v\n%s", err, b)
	}
	services, _ := doc["services"].(map[string]any)
	svc, _ := services[name].(map[string]any)
	if svc == nil {
		t.Fatalf("service %q missing from export:\n%s", name, b)
	}
	return svc, string(b)
}

func TestRoundTripLongSyntaxPortsAndVolumes(t *testing.T) {
	cfg := importString(t, `
services:
  web:
    image: nginx:1.27
    ports:
      - target: 80
        published: 8080
        host_ip: 127.0.0.1
    volumes:
      - type: bind
        source: ./site
        target: /usr/share/nginx/html
        read_only: true
`)
	svc, out := exportedService(t, cfg, "web")
	ports, _ := svc["ports"].([]any)
	if len(ports) != 1 {
		t.Fatalf("ports = %v, want one long-syntax entry\n%s", svc["ports"], out)
	}
	if entry, _ := ports[0].(map[string]any); entry["host_ip"] != "127.0.0.1" {
		t.Fatalf("long-syntax port lost its fields: %v", ports[0])
	}
	vols, _ := svc["volumes"].([]any)
	if len(vols) != 1 {
		t.Fatalf("volumes = %v, want one long-syntax entry", svc["volumes"])
	}
	if entry, _ := vols[0].(map[string]any); entry["read_only"] != true {
		t.Fatalf("long-syntax volume lost read_only: %v", vols[0])
	}
}

func TestRoundTripMixedShortAndLongPorts(t *testing.T) {
	cfg := importString(t, `
services:
  web:
    image: nginx:1.27
    ports:
      - "8080:80"
      - target: 443
        published: 8443
`)
	svc := cfg.GetService("web")
	if len(svc.Ports) != 1 || svc.Ports[0] != "8080:80" {
		t.Fatalf("Ports = %v, want the short entry only", svc.Ports)
	}
	out, _ := exportedService(t, cfg, "web")
	if ports, _ := out["ports"].([]any); len(ports) != 2 {
		t.Fatalf("exported ports = %v, want both entries", out["ports"])
	}
}

// Adding a short-syntax port through the form to a service that was
// imported with long-syntax ports used to emit `ports:` twice.
func TestFormEditOnLongSyntaxPortsDoesNotDuplicateKey(t *testing.T) {
	cfg := importString(t, `
services:
  web:
    image: nginx:1.27
    ports:
      - target: 80
        published: 8080
`)
	svc := cfg.GetService("web")
	svc.Ports = append(svc.Ports, "9090:90")
	out, _ := exportedService(t, cfg, "web")
	if ports, _ := out["ports"].([]any); len(ports) != 2 {
		t.Fatalf("ports = %v, want the new short entry and the original long one", out["ports"])
	}
}

func TestBuildSetThroughFormReplacesBuildMapping(t *testing.T) {
	cfg := importString(t, `
services:
  app:
    build:
      context: ./app
      dockerfile: Dockerfile.dev
`)
	svc := cfg.GetService("app")
	if svc.Build != "" || !svc.HasBuild() {
		t.Fatalf("build mapping should live in Extra and still count as a build")
	}
	svc.Build = "./other"
	out, _ := exportedService(t, cfg, "app")
	if out["build"] != "./other" {
		t.Fatalf("build = %v, want the form value to win", out["build"])
	}
}

func TestHealthcheckKeepsStartPeriod(t *testing.T) {
	cfg := importString(t, `
services:
  db:
    image: postgres:16
    healthcheck:
      test: ["CMD", "pg_isready"]
      interval: 10s
      start_period: 40s
      start_interval: 2s
`)
	hc := cfg.GetService("db").HealthCheck
	if hc == nil || hc.StartPeriod != "40s" || hc.StartInterval != "2s" {
		t.Fatalf("HealthCheck = %+v, want start_period/start_interval kept", hc)
	}
	out, _ := exportedService(t, cfg, "db")
	exported, _ := out["healthcheck"].(map[string]any)
	if exported["start_period"] != "40s" || exported["start_interval"] != "2s" {
		t.Fatalf("exported healthcheck = %v", exported)
	}
}

func TestHealthcheckWithUnmodeledShapesStaysVerbatim(t *testing.T) {
	cases := map[string]string{
		"disable":     "healthcheck:\n      disable: true\n",
		"no test key": "healthcheck:\n      interval: 5s\n",
		"unknown key": "healthcheck:\n      test: [\"CMD\", \"true\"]\n      x-note: hi\n",
	}
	for name, block := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := importString(t, "services:\n  db:\n    image: postgres:16\n    "+block)
			svc := cfg.GetService("db")
			if svc.HealthCheck != nil {
				t.Fatalf("HealthCheck = %+v, want it kept verbatim in Extra", svc.HealthCheck)
			}
			if !svc.HasHealthCheck() {
				t.Fatal("HasHealthCheck = false, want true")
			}
			out, _ := exportedService(t, cfg, "db")
			hc, _ := out["healthcheck"].(map[string]any)
			if hc == nil {
				t.Fatalf("healthcheck dropped on export")
			}
			if _, hasTest := hc["test"]; name == "no test key" && hasTest {
				t.Fatalf("export invented a test key: %v", hc)
			}
			r := cfg.Validate()
			if hasError(r, "healthcheck.test") || hasWarning(r, "healthcheck") {
				t.Fatalf("healthcheck in Extra must not be reported as missing: %+v", r)
			}
		})
	}
}

func TestEnvironmentMapEmptyStringIsNotPassThrough(t *testing.T) {
	cfg := importString(t, `
services:
  web:
    image: nginx:1.27
    environment:
      EMPTY: ""
      PASSTHROUGH:
      SET: x
`)
	got := strings.Join(cfg.GetService("web").Environment, ",")
	if want := "EMPTY=,PASSTHROUGH,SET=x"; got != want {
		t.Fatalf("Environment = %q, want %q", got, want)
	}
}

func TestExportIsDeterministic(t *testing.T) {
	cfg := importString(t, `
x-a: 1
x-b: 2
x-c: 3
services:
  web:
    image: nginx:1.27
    command: ["nginx"]
    container_name: web
    labels: {a: b}
    networks: [front]
    working_dir: /app
`)
	first, err := cfg.ExportYAML()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 25; i++ {
		again, _ := cfg.ExportYAML()
		if string(again) != string(first) {
			t.Fatalf("export differs between runs:\n%s\n---\n%s", first, again)
		}
	}
}

func TestValidateBuildMappingWithoutImageIsFine(t *testing.T) {
	cfg := importString(t, `
services:
  app:
    build:
      context: ./app
`)
	if r := cfg.Validate(); hasError(r, "image/build") {
		t.Fatalf("build mapping must satisfy image-or-build, got %+v", r.Errors)
	}
}

func TestValidateSeesDockerSocketInLongSyntaxVolume(t *testing.T) {
	cfg := importString(t, `
services:
  agent:
    image: portainer/portainer-ce:2.21.0
    volumes:
      - type: bind
        source: /var/run/docker.sock
        target: /var/run/docker.sock
`)
	if r := cfg.Validate(); !hasWarning(r, "volumes") {
		t.Fatalf("expected a docker socket warning for a long-syntax mount, got %+v", r.Warnings)
	}
}

func TestValidateSeesExposedLongSyntaxPort(t *testing.T) {
	cfg := importString(t, `
services:
  web:
    image: nginx:1.27
    ports:
      - target: 80
        published: 8080
`)
	if r := cfg.Validate(); !hasWarning(r, "ports") {
		t.Fatalf("expected an all-interfaces warning for a long-syntax port, got %+v", r.Warnings)
	}
	loopback := importString(t, `
services:
  web:
    image: nginx:1.27
    ports:
      - target: 80
        published: 8080
        host_ip: 127.0.0.1
`)
	if r := loopback.Validate(); hasWarning(r, "ports") {
		t.Fatalf("loopback-bound long-syntax port must not warn, got %+v", r.Warnings)
	}
}

func TestValidateLongSyntaxVolumeCountsAsVolume(t *testing.T) {
	cfg := importString(t, `
services:
  db:
    image: postgres:16
    volumes:
      - type: volume
        source: pgdata
        target: /var/lib/postgresql/data
`)
	if r := cfg.Validate(); hasWarning(r, "volumes") {
		t.Fatalf("a long-syntax volume must satisfy the database-needs-a-volume check, got %+v", r.Warnings)
	}
}

func TestValidateServiceHealthyAcceptsUnmodeledHealthcheck(t *testing.T) {
	cfg := importString(t, `
services:
  db:
    image: postgres:16
    healthcheck:
      test: pg_isready
  app:
    image: myapp:1.0
    depends_on:
      db:
        condition: service_healthy
`)
	if r := cfg.Validate(); hasError(r, "depends_on") {
		t.Fatalf("string-form healthcheck satisfies service_healthy, got %+v", r.Errors)
	}
}
