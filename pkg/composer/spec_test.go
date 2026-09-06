package composer

import "testing"

func TestValidateYAMLAgainstSpec_ValidDoc(t *testing.T) {
	yamlDoc := []byte(`
services:
  web:
    image: nginx:latest
    ports:
      - "8080:80"
`)
	result, err := ValidateYAMLAgainstSpec(yamlDoc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Valid {
		t.Fatalf("expected valid doc to pass spec validation, got issues: %v", result.Issues)
	}
}

func TestValidateYAMLAgainstSpec_InvalidDoc(t *testing.T) {
	// "ports" must be an array per the spec; a bare string here should
	// fail schema validation even though it's syntactically valid YAML.
	yamlDoc := []byte(`
services:
  web:
    image: nginx:latest
    ports: "not-an-array"
`)
	result, err := ValidateYAMLAgainstSpec(yamlDoc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Valid {
		t.Fatalf("expected invalid doc to fail spec validation")
	}
	if len(result.Issues) == 0 {
		t.Fatalf("expected at least one issue for invalid doc")
	}
}

func TestValidateYAMLAgainstSpec_UnknownTopLevelKey(t *testing.T) {
	yamlDoc := []byte(`
services:
  web:
    image: nginx:latest
totally_not_a_compose_key: true
`)
	result, err := ValidateYAMLAgainstSpec(yamlDoc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Valid {
		t.Fatalf("expected doc with unknown top-level key to fail spec validation")
	}
}
