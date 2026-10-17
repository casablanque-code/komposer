package check

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	cleanYAML = `services:
  web:
    image: nginx:1.27
    ports:
      - "127.0.0.1:8080:80"
`
	warnYAML = `services:
  web:
    image: nginx:1.27
    ports:
      - "8080:80"
`
	errorYAML = `services:
  web:
    restart: always
`
	specBadYAML = `services:
  web:
    image: nginx:1.27
unknown-top-level-key: 1
`
)

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	return p
}

func run(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errb bytes.Buffer
	code = Run(args, &out, &errb, PlainStyler())
	return code, out.String(), errb.String()
}

func inDir(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
}

func TestCleanFilePasses(t *testing.T) {
	p := writeFile(t, t.TempDir(), "compose.yaml", cleanYAML)
	code, out, _ := run(t, p)
	if code != ExitOK {
		t.Fatalf("exit = %d, want 0\n%s", code, out)
	}
	for _, want := range []string{"✓ Errors: none", "✓ Warnings: none", "✓ Compose specification: valid", "PASS"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestErrorsFail(t *testing.T) {
	p := writeFile(t, t.TempDir(), "compose.yaml", errorYAML)
	code, out, _ := run(t, p)
	if code != ExitFail {
		t.Fatalf("exit = %d, want 1\n%s", code, out)
	}
	if !strings.Contains(out, "✗ Errors (1)") || !strings.Contains(out, "FAIL — 1 error") {
		t.Errorf("unexpected output:\n%s", out)
	}
}

func TestWarningsOnlyPassUnlessStrict(t *testing.T) {
	p := writeFile(t, t.TempDir(), "compose.yaml", warnYAML)

	code, out, _ := run(t, p)
	if code != ExitOK {
		t.Fatalf("exit = %d, want 0 without --strict\n%s", code, out)
	}
	if !strings.Contains(out, "⚠ Warnings (1)") || !strings.Contains(out, "use --strict") {
		t.Errorf("unexpected output:\n%s", out)
	}

	for _, args := range [][]string{{"--strict", p}, {p, "--strict"}} {
		code, out, _ = run(t, args...)
		if code != ExitFail {
			t.Fatalf("%v: exit = %d, want 1 with --strict\n%s", args, code, out)
		}
		if !strings.Contains(out, "FAIL — 1 warning (--strict)") {
			t.Errorf("unexpected output:\n%s", out)
		}
	}
}

func TestSpecViolationFails(t *testing.T) {
	p := writeFile(t, t.TempDir(), "compose.yaml", specBadYAML)
	code, out, _ := run(t, p)
	if code != ExitFail {
		t.Fatalf("exit = %d, want 1\n%s", code, out)
	}
	if !strings.Contains(out, "✗ Compose specification") {
		t.Errorf("unexpected output:\n%s", out)
	}
}

func TestUnparseableYAMLFails(t *testing.T) {
	p := writeFile(t, t.TempDir(), "compose.yaml", "services: [unclosed\n")
	code, out, _ := run(t, p)
	if code != ExitFail {
		t.Fatalf("exit = %d, want 1\n%s", code, out)
	}
	if !strings.Contains(out, "✗ Compose specification") {
		t.Errorf("unexpected output:\n%s", out)
	}
}

func TestJSONOutput(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "compose.yaml", warnYAML)

	for _, args := range [][]string{{"--format", "json", p}, {"--format=json", p}, {p, "--format", "json"}} {
		code, out, _ := run(t, args...)
		if code != ExitOK {
			t.Fatalf("%v: exit = %d", args, code)
		}
		var rep Report
		if err := json.Unmarshal([]byte(out), &rep); err != nil {
			t.Fatalf("%v: output is not JSON: %v\n%s", args, err, out)
		}
		if !rep.Passed || len(rep.Warnings) != 1 || rep.Warnings[0].Field != "ports" || rep.File != p {
			t.Errorf("%v: unexpected report %+v", args, rep)
		}
	}

	// Empty lists must be [] (not null) so consumers can iterate them.
	_, out, _ := run(t, "--format=json", writeFile(t, dir, "ok.yaml", cleanYAML))
	for _, want := range []string{`"errors": []`, `"warnings": []`, `"issues": []`} {
		if !strings.Contains(out, want) {
			t.Errorf("JSON missing %s:\n%s", want, out)
		}
	}

	// --strict is reflected in passed.
	code, out, _ := run(t, "--format=json", "--strict", p)
	if code != ExitFail || !strings.Contains(out, `"passed": false`) || !strings.Contains(out, `"strict": true`) {
		t.Errorf("exit = %d, output:\n%s", code, out)
	}
}

func TestDefaultFileDiscovery(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "docker-compose.yml", errorYAML)
	writeFile(t, dir, "compose.yaml", cleanYAML) // wins, as in Docker Compose
	inDir(t, dir)

	code, out, _ := run(t)
	if code != ExitOK || !strings.Contains(out, "compose.yaml") {
		t.Fatalf("exit = %d, want compose.yaml to be picked and pass\n%s", code, out)
	}
}

func TestUsageErrors(t *testing.T) {
	dir := t.TempDir()
	ok := writeFile(t, dir, "compose.yaml", cleanYAML)
	cases := map[string][]string{
		"unknown flag":  {"--nope", ok},
		"two files":     {ok, ok},
		"bad format":    {"--format", "xml", ok},
		"format needs":  {"--format"},
		"missing file":  {filepath.Join(dir, "absent.yaml")},
		"directory arg": {dir},
	}
	for name, args := range cases {
		code, out, errOut := run(t, args...)
		if code != ExitUsage {
			t.Errorf("%s: exit = %d, want 2", name, code)
		}
		if out != "" {
			t.Errorf("%s: stdout should be empty, got %q", name, out)
		}
		if !strings.Contains(errOut, "komposer check:") {
			t.Errorf("%s: stderr = %q", name, errOut)
		}
	}

	inDir(t, t.TempDir())
	code, _, errOut := run(t)
	if code != ExitUsage || !strings.Contains(errOut, "no compose file found") {
		t.Errorf("no default file: exit = %d, stderr = %q", code, errOut)
	}
}

func TestHelp(t *testing.T) {
	for _, flag := range []string{"-h", "--help"} {
		code, out, _ := run(t, flag)
		if code != ExitOK || !strings.Contains(out, "Usage: komposer check") || !strings.Contains(out, "--strict") {
			t.Errorf("%s: exit = %d, output:\n%s", flag, code, out)
		}
	}
}

func TestStylerIsApplied(t *testing.T) {
	p := writeFile(t, t.TempDir(), "compose.yaml", errorYAML)
	var out, errb bytes.Buffer
	mark := func(tag string) func(string) string {
		return func(s string) string { return "<" + tag + ">" + s + "</" + tag + ">" }
	}
	Run([]string{p}, &out, &errb, Styler{Error: mark("e"), OK: mark("o"), Warn: mark("w"), Bold: mark("b")})
	if !strings.Contains(out.String(), "<e>✗ Errors (1)</e>") || !strings.Contains(out.String(), "<o>✓ Warnings: none</o>") {
		t.Errorf("styler not applied:\n%s", out.String())
	}
}

func TestFindingsShowRuleIDs(t *testing.T) {
	p := writeFile(t, t.TempDir(), "compose.yaml", warnYAML)

	_, out, _ := run(t, p)
	if !strings.Contains(out, "[port-all-interfaces] service 'web': ports:") {
		t.Errorf("text output should lead each finding with its rule ID:\n%s", out)
	}

	_, out, _ = run(t, "--format=json", p)
	var rep Report
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatal(err)
	}
	if len(rep.Warnings) != 1 || rep.Warnings[0].Rule != "port-all-interfaces" {
		t.Errorf("JSON warning should carry its rule: %+v", rep.Warnings)
	}

	e := writeFile(t, t.TempDir(), "compose.yaml", errorYAML)
	_, out, _ = run(t, "--format=json", e)
	rep = Report{}
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatal(err)
	}
	if len(rep.Errors) != 1 || rep.Errors[0].Rule != "image-or-build-required" {
		t.Errorf("JSON error should carry its rule: %+v", rep.Errors)
	}
}
