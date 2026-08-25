package composer

import (
	"strings"
	"testing"
)

// TestExportYAMLUsesTwoSpaceIndent guards against a regression to
// yaml.v3's package-level Marshal, whose default indent is 4 spaces —
// see ExportYAML's doc comment. Every nesting level here is checked
// explicitly so a regression shows exactly which level broke.
func TestExportYAMLUsesTwoSpaceIndent(t *testing.T) {
	c := NewComposeConfig()
	svc := c.AddService("web")
	svc.Image = "nginx:alpine"
	svc.Ports = []string{"80:80"}
	svc.Environment = []string{"FOO=bar"}

	out, err := c.ExportYAML()
	if err != nil {
		t.Fatalf("ExportYAML: %v", err)
	}

	want := "services:\n" +
		"  web:\n" +
		"    image: nginx:alpine\n" +
		"    ports:\n" +
		"      - 80:80\n" +
		"    environment:\n" +
		"      - FOO=bar\n"
	if got := string(out); got != want {
		t.Fatalf("ExportYAML indentation mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestExportYAMLOmitsVersionWhenUnset(t *testing.T) {
	c := NewComposeConfig()
	c.AddService("web").Image = "nginx"

	out, err := c.ExportYAML()
	if err != nil {
		t.Fatalf("ExportYAML: %v", err)
	}
	if strings.Contains(string(out), "version:") {
		t.Fatalf("ExportYAML should omit the deprecated top-level version key when unset, got:\n%s", out)
	}
}

func TestExportYAMLPreservesVersionWhenSet(t *testing.T) {
	c := NewComposeConfig()
	c.Version = "3.9"
	c.AddService("web").Image = "nginx"

	out, err := c.ExportYAML()
	if err != nil {
		t.Fatalf("ExportYAML: %v", err)
	}
	if !strings.HasPrefix(string(out), "version: \"3.9\"\n") {
		t.Fatalf("expected version to be preserved as the first line, got:\n%s", out)
	}
}

func TestExportYAMLServiceOrderIsInsertionOrder(t *testing.T) {
	c := NewComposeConfig()
	c.AddService("zebra").Image = "a"
	c.AddService("apple").Image = "b"
	c.AddService("mango").Image = "c"

	out, err := c.ExportYAML()
	if err != nil {
		t.Fatalf("ExportYAML: %v", err)
	}
	s := string(out)
	iz, ia, im := strings.Index(s, "zebra:"), strings.Index(s, "apple:"), strings.Index(s, "mango:")
	if !(iz < ia && ia < im) {
		t.Fatalf("expected services in insertion order (zebra, apple, mango), got:\n%s", s)
	}
}

func TestExportYAMLDependsOnLongForm(t *testing.T) {
	c := NewComposeConfig()
	c.AddService("db").Image = "postgres"
	web := c.AddService("web")
	web.Image = "nginx"
	web.DependsOn = []DependsOnEntry{{Service: "db", Condition: CondServiceHealthy}}

	out, err := c.ExportYAML()
	if err != nil {
		t.Fatalf("ExportYAML: %v", err)
	}
	want := "    depends_on:\n" +
		"      db:\n" +
		"        condition: service_healthy\n"
	if !strings.Contains(string(out), want) {
		t.Fatalf("expected depends_on long form in output, got:\n%s", out)
	}
}
