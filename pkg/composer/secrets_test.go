package composer

import (
	"strings"
	"testing"
)

func buildTestConfigWithSecret(envValue string) *ComposeConfig {
	c := NewComposeConfig()
	svc := c.AddService("db")
	svc.Image = "postgres:16"
	svc.Environment = []string{"POSTGRES_PASSWORD=" + envValue, "POSTGRES_USER=app"}
	return c
}

func TestFindHardcodedSecrets(t *testing.T) {
	c := buildTestConfigWithSecret("hunter2")
	found := c.FindHardcodedSecrets()
	if len(found) != 1 {
		t.Fatalf("expected 1 hardcoded secret, got %d: %+v", len(found), found)
	}
	if found[0].Service != "db" || found[0].Key != "POSTGRES_PASSWORD" || found[0].Empty {
		t.Fatalf("unexpected result: %+v", found[0])
	}
}

func TestFindHardcodedSecrets_EmptyValue(t *testing.T) {
	c := buildTestConfigWithSecret("")
	found := c.FindHardcodedSecrets()
	if len(found) != 1 || !found[0].Empty {
		t.Fatalf("expected 1 empty secret, got %+v", found)
	}
}

func TestFindHardcodedSecrets_SubstitutionIsFine(t *testing.T) {
	c := buildTestConfigWithSecret("${POSTGRES_PASSWORD}")
	found := c.FindHardcodedSecrets()
	if len(found) != 0 {
		t.Fatalf("expected substitution to not be flagged, got %+v", found)
	}
}

func TestConvertSecretToEnvFile(t *testing.T) {
	c := buildTestConfigWithSecret("hunter2")
	line, err := c.ConvertSecretToEnvFile("db", "POSTGRES_PASSWORD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if line != "POSTGRES_PASSWORD=hunter2" {
		t.Fatalf("unexpected .env line: %q", line)
	}

	svc := c.GetService("db")
	idx, value, found := findEnvEntry(svc.Environment, "POSTGRES_PASSWORD")
	if !found || idx != 0 || value != "${POSTGRES_PASSWORD}" {
		t.Fatalf("expected environment entry rewritten to substitution, got idx=%d value=%q found=%v", idx, value, found)
	}

	// The rewritten entry must no longer be flagged.
	if found := c.FindHardcodedSecrets(); len(found) != 0 {
		t.Fatalf("expected no more hardcoded secrets after conversion, got %+v", found)
	}
}

func TestConvertSecretToEnvFile_UnknownService(t *testing.T) {
	c := buildTestConfigWithSecret("hunter2")
	if _, err := c.ConvertSecretToEnvFile("nope", "POSTGRES_PASSWORD"); err == nil {
		t.Fatalf("expected error for unknown service")
	}
}

func TestConvertSecretToEnvFile_UnknownKey(t *testing.T) {
	c := buildTestConfigWithSecret("hunter2")
	if _, err := c.ConvertSecretToEnvFile("db", "NOT_A_KEY"); err == nil {
		t.Fatalf("expected error for unknown key")
	}
}

func TestConvertSecretToComposeSecret(t *testing.T) {
	c := buildTestConfigWithSecret("hunter2")
	secretName, filePath, value, err := c.ConvertSecretToComposeSecret("db", "POSTGRES_PASSWORD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if secretName != "db_postgres_password" {
		t.Fatalf("unexpected secret name: %q", secretName)
	}
	if filePath != "./secrets/db_postgres_password.txt" {
		t.Fatalf("unexpected file path: %q", filePath)
	}
	if value != "hunter2" {
		t.Fatalf("unexpected value: %q", value)
	}

	svc := c.GetService("db")
	if len(svc.Environment) != 1 || !strings.HasPrefix(svc.Environment[0], "POSTGRES_USER=") {
		t.Fatalf("expected the secret's env entry to be removed, got %+v", svc.Environment)
	}

	// Top-level secrets: block should now exist with the right file.
	secretsNode, ok := c.Extra["secrets"]
	if !ok {
		t.Fatalf("expected top-level 'secrets:' block to be created")
	}
	if !secretsMappingHasKey(&secretsNode, secretName) {
		t.Fatalf("expected top-level secrets to contain %q", secretName)
	}

	// Service-level secrets: reference should exist.
	svcSecretsNode, ok := svc.Extra["secrets"]
	if !ok {
		t.Fatalf("expected service-level 'secrets:' list to be created")
	}
	found := false
	for _, item := range svcSecretsNode.Content {
		if item.Value == secretName {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected service secrets list to reference %q", secretName)
	}

	// Exporting should not error and should contain the new secret.
	out, err := c.ExportYAML()
	if err != nil {
		t.Fatalf("unexpected export error: %v", err)
	}
	if !strings.Contains(string(out), "db_postgres_password") {
		t.Fatalf("expected exported YAML to reference the new secret, got:\n%s", out)
	}
	if strings.Contains(string(out), "hunter2") {
		t.Fatalf("secret value must never be written into the compose file itself, got:\n%s", out)
	}
}

func TestConvertSecretToComposeSecret_MergesWithExistingBlock(t *testing.T) {
	c := buildTestConfigWithSecret("hunter2")

	// Simulate a pre-existing top-level secrets: block, as if imported
	// from a real file with an unrelated secret already defined.
	if err := mergeTopLevelSecret(c, "other_secret", "./secrets/other_secret.txt"); err != nil {
		t.Fatalf("setup: %v", err)
	}

	_, _, _, err := c.ConvertSecretToComposeSecret("db", "POSTGRES_PASSWORD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	secretsNode := c.Extra["secrets"]
	if !secretsMappingHasKey(&secretsNode, "other_secret") {
		t.Fatalf("expected pre-existing secret to survive the merge")
	}
	if !secretsMappingHasKey(&secretsNode, "db_postgres_password") {
		t.Fatalf("expected new secret to be added alongside the existing one")
	}
}

func TestConvertSecretToComposeSecret_NoDuplicateOnSecondCall(t *testing.T) {
	c := buildTestConfigWithSecret("hunter2")
	svc := c.GetService("db")
	// Add a second service sharing the same generated secret name by
	// coincidence (unlikely in practice, but the merge logic should
	// still not produce a duplicate key or a duplicate reference).
	svc.Environment = append(svc.Environment, "POSTGRES_PASSWORD_2=hunter2")

	if _, _, _, err := c.ConvertSecretToComposeSecret("db", "POSTGRES_PASSWORD"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mergeTopLevelSecret(c, "db_postgres_password", "./secrets/db_postgres_password.txt"); err != nil {
		t.Fatalf("unexpected error re-merging same secret: %v", err)
	}
	if err := mergeServiceSecretRef(svc, "db_postgres_password"); err != nil {
		t.Fatalf("unexpected error re-referencing same secret: %v", err)
	}

	secretsNode := c.Extra["secrets"]
	count := 0
	for i := 0; i+1 < len(secretsNode.Content); i += 2 {
		if secretsNode.Content[i].Value == "db_postgres_password" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly one 'db_postgres_password' key, got %d", count)
	}

	refs := 0
	for _, item := range svc.Extra["secrets"].Content {
		if item.Value == "db_postgres_password" {
			refs++
		}
	}
	if refs != 1 {
		t.Fatalf("expected exactly one reference to 'db_postgres_password', got %d", refs)
	}
}
