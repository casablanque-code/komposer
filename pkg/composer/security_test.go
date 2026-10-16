package composer

import (
	"strings"
	"testing"
)

// warningsFor returns the warning messages for field on a service of the
// imported document.
func warningsFor(t *testing.T, doc, service, field string) []string {
	t.Helper()
	cfg := importString(t, doc)
	var out []string
	for _, w := range cfg.Validate().Warnings {
		if w.Service == service && w.Field == field {
			out = append(out, w.Message)
		}
	}
	return out
}

func TestSensitiveMounts(t *testing.T) {
	cases := []struct {
		name string
		vol  string
		warn bool
	}{
		{"etc rw", `"/etc:/host-etc"`, true},
		{"root rw", `"/:/host"`, true},
		{"docker lib rw", `"/var/lib/docker:/var/lib/docker"`, true},
		{"trailing slash", `"/etc/:/host-etc"`, true},
		{"etc ro", `"/etc:/host-etc:ro"`, false},
		{"ro with selinux flag", `"/etc:/host-etc:ro,z"`, false},
		{"root ro (monitoring pattern)", `"/:/rootfs:ro"`, false},
		{"single file", `"/etc/localtime:/etc/localtime:ro"`, false},
		{"file rw is not the dir", `"/etc/myapp.conf:/etc/app.conf"`, false},
		{"relative bind", `"./data:/data"`, false},
		{"named volume", `"pgdata:/var/lib/postgresql/data"`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := warningsFor(t, "services:\n  app:\n    image: myapp:1.0\n    volumes:\n      - "+tc.vol+"\n", "app", "volumes")
			if (len(got) > 0) != tc.warn {
				t.Fatalf("warnings = %v, want warn=%v", got, tc.warn)
			}
		})
	}
}

func TestSensitiveMountsLongSyntax(t *testing.T) {
	rw := `
services:
  app:
    image: myapp:1.0
    volumes:
      - type: bind
        source: /etc
        target: /host-etc
`
	if got := warningsFor(t, rw, "app", "volumes"); len(got) != 1 {
		t.Fatalf("read-write long-syntax bind of /etc: warnings = %v", got)
	}
	ro := strings.Replace(rw, "target: /host-etc", "target: /host-etc\n        read_only: true", 1)
	if got := warningsFor(t, ro, "app", "volumes"); len(got) != 0 {
		t.Fatalf("read-only long-syntax bind must not warn: %v", got)
	}
	named := `
services:
  app:
    image: myapp:1.0
    volumes:
      - type: volume
        source: /etc
        target: /x
`
	if got := warningsFor(t, named, "app", "volumes"); len(got) != 0 {
		t.Fatalf("non-bind mount must not warn: %v", got)
	}
}

func TestRollingTags(t *testing.T) {
	cases := map[string]bool{
		"nginx:stable":                          true,
		"nginx:stable-alpine":                   true,
		"app:main":                              true,
		"app:main-slim":                         true,
		"app:Nightly":                           true,
		"app:latest-alpine":                     true,
		"nginx:1.27":                            false,
		"nginx:1.27-alpine":                     false,
		"app:nightly-20241001":                  false,
		"app:main-3f2a9c1":                      false,
		"app:v2":                                false,
		"app@sha256:" + strings.Repeat("a", 64): false,
	}
	for image, want := range cases {
		doc := "services:\n  app:\n    image: " + image + "\n"
		cfg := importString(t, doc)
		got := false
		for _, w := range cfg.Validate().Warnings {
			if w.Field == "image" && strings.Contains(w.Message, "rolling tag") {
				got = true
			}
		}
		if got != want {
			t.Errorf("%s: rolling-tag warning = %v, want %v", image, got, want)
		}
	}
}

func TestLatestKeepsItsOwnMessage(t *testing.T) {
	got := warningsFor(t, "services:\n  app:\n    image: nginx:latest\n", "app", "image")
	if len(got) != 1 || strings.Contains(got[0], "rolling tag") {
		t.Fatalf("':latest' must keep exactly its dedicated warning, got %v", got)
	}
}

func TestHostNamespacesAndNoNewPrivileges(t *testing.T) {
	doc := `
services:
  a:
    image: app:1.0
    ipc: host
    userns_mode: host
    cgroup: host
    security_opt:
      - no-new-privileges:false
  b:
    image: app:1.0
    ipc: "service:a"
    security_opt:
      - no-new-privileges:true
`
	cfg := importString(t, doc)
	r := cfg.Validate()
	for _, field := range []string{"ipc", "userns_mode", "cgroup", "security_opt"} {
		found := false
		for _, w := range r.Warnings {
			if w.Service == "a" && w.Field == field {
				found = true
			}
			if w.Service == "b" {
				t.Errorf("service b is fine but got warning %+v", w)
			}
		}
		if !found {
			t.Errorf("service a: missing %s warning; got %+v", field, r.Warnings)
		}
	}
}

func TestDatabaseAuth(t *testing.T) {
	cases := []struct {
		name string
		doc  string
		want int // warnings on field "environment"
	}{
		{"postgres exposed, no password", "image: postgres:16\n    ports: [\"5432:5432\"]", 1},
		{"postgres exposed, password set", "image: postgres:16\n    ports: [\"5432:5432\"]\n    environment:\n      POSTGRES_PASSWORD: ${PG_PW}", 0},
		{"postgres exposed, password via _FILE", "image: postgres:16\n    ports: [\"5432:5432\"]\n    environment:\n      POSTGRES_PASSWORD_FILE: /run/secrets/pg", 0},
		{"bitnami postgres password", "image: bitnami/postgresql:16\n    ports: [\"5432:5432\"]\n    environment:\n      POSTGRESQL_PASSWORD: ${PG_PW}", 0},
		{"postgres exposed, env_file hides it", "image: postgres:16\n    ports: [\"5432:5432\"]\n    env_file: .env", 0},
		{"postgres loopback only", "image: postgres:16\n    ports: [\"127.0.0.1:5432:5432\"]", 0},
		{"postgres not published", "image: postgres:16", 0},
		{"postgres trust, not exposed", "image: postgres:16\n    environment:\n      POSTGRES_PASSWORD: ${PG_PW}\n      POSTGRES_HOST_AUTH_METHOD: trust", 1},
		{"postgres exporter is not postgres", "image: prometheuscommunity/postgres-exporter:v0.15.0\n    ports: [\"9187:9187\"]", 0},
		{"mongo exposed, no auth", "image: mongo:7\n    ports: [\"27017:27017\"]", 1},
		{"mongo exposed, root user set", "image: mongo:7\n    ports: [\"27017:27017\"]\n    environment:\n      MONGO_INITDB_ROOT_USERNAME: root\n      MONGO_INITDB_ROOT_PASSWORD: ${MONGO_PW}", 0},
		{"mongo exposed, --auth", "image: mongo:7\n    ports: [\"27017:27017\"]\n    command: [\"mongod\", \"--auth\"]", 0},
		{"mongo not published", "image: mongo:7", 0},
		{"mysql allow empty", "image: mysql:8\n    environment:\n      MYSQL_ALLOW_EMPTY_PASSWORD: \"yes\"", 1},
		{"mariadb allow empty", "image: mariadb:11\n    environment:\n      MARIADB_ALLOW_EMPTY_ROOT_PASSWORD: \"1\"", 1},
		{"mysql allow empty = no", "image: mysql:8\n    environment:\n      MYSQL_ALLOW_EMPTY_PASSWORD: \"no\"", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := warningsFor(t, "services:\n  db:\n    "+tc.doc+"\n", "db", "environment")
			// "looks like a secret" warnings are a separate, older check;
			// count only the database-auth ones.
			n := 0
			for _, m := range got {
				if !strings.Contains(m, "looks like a secret") {
					n++
				}
			}
			if n != tc.want {
				t.Fatalf("database-auth warnings = %d (%v), want %d", n, got, tc.want)
			}
		})
	}
}

func TestDependencyCycles(t *testing.T) {
	cfg := importString(t, `
services:
  a:
    image: app:1.0
    depends_on: [b]
  b:
    image: app:1.0
    depends_on: [c]
  c:
    image: app:1.0
    depends_on: [a]
  d:
    image: app:1.0
    depends_on: [d]
  ok:
    image: app:1.0
    depends_on: [a]
`)
	var cycles []string
	for _, e := range cfg.Validate().Errors {
		if e.Field == "depends_on" && strings.Contains(e.Message, "dependency cycle") {
			cycles = append(cycles, e.Message)
		}
	}
	if len(cycles) != 2 {
		t.Fatalf("want exactly 2 cycles (a→b→c→a and d→d), got %d: %v", len(cycles), cycles)
	}
	joined := strings.Join(cycles, "\n")
	for _, want := range []string{"a → b → c → a", "d → d"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing cycle %q in %v", want, cycles)
		}
	}
}

func TestNoCycleInDiamond(t *testing.T) {
	cfg := importString(t, `
services:
  app:
    image: app:1.0
    depends_on: [api, worker]
  api:
    image: app:1.0
    depends_on: [db]
  worker:
    image: app:1.0
    depends_on: [db]
  db:
    image: app:1.0
`)
	for _, e := range cfg.Validate().Errors {
		if strings.Contains(e.Message, "cycle") {
			t.Fatalf("diamond-shaped dependencies are not a cycle: %+v", e)
		}
	}
}

func TestCycleThroughMissingServiceDoesNotPanic(t *testing.T) {
	cfg := importString(t, `
services:
  a:
    image: app:1.0
    depends_on: [ghost]
`)
	r := cfg.Validate()
	if !hasError(r, "depends_on") {
		t.Fatalf("missing service must still be reported: %+v", r.Errors)
	}
}
