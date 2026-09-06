package composer

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// HardcodedSecret identifies one environment entry that
// classifySecretEnv flagged as empty or hardcoded — the same
// detection Validate() uses for its advisory warnings, exposed here in
// structured form (service + key) instead of a pre-formatted message,
// so callers (e.g. a TUI action bound to a specific warning) can act
// on a specific one instead of re-parsing the warning text.
type HardcodedSecret struct {
	Service string
	Key     string
	Empty   bool // true if the value is unset; false if it's hardcoded
}

// FindHardcodedSecrets scans every service's environment for entries
// classifySecretEnv considers a footgun (empty or hardcoded, by
// variable-name convention) and returns them in service/declaration
// order.
func (c *ComposeConfig) FindHardcodedSecrets() []HardcodedSecret {
	var found []HardcodedSecret
	for _, entry := range c.Services {
		if entry.Config == nil {
			continue
		}
		for _, env := range entry.Config.Environment {
			key, empty, hardcoded := classifySecretEnv(env)
			if empty || hardcoded {
				found = append(found, HardcodedSecret{Service: entry.Name, Key: key, Empty: empty})
			}
		}
	}
	return found
}

// findEnvEntry locates the "environment:" entry for the given key
// (case-insensitive, matching classifySecretEnv/hasNonEmptyEnv) and
// returns its index and current value.
func findEnvEntry(env []string, key string) (idx int, value string, found bool) {
	for i, entry := range env {
		eq := strings.Index(entry, "=")
		if eq < 0 {
			continue
		}
		k := strings.TrimSpace(entry[:eq])
		if strings.EqualFold(k, key) {
			return i, strings.TrimSpace(entry[eq+1:]), true
		}
	}
	return -1, "", false
}

// ConvertSecretToEnvFile rewrites a hardcoded (or empty) secret
// environment entry on the given service into "${KEY}" substitution —
// classifySecretEnv already treats "${...}" values as fine, so this is
// what makes the warning go away — and returns the "KEY=value" line
// the caller should append to a ".env" file next to the compose file
// (docker compose reads ".env" automatically for variable
// substitution; writing that file is left to the caller, since
// pkg/composer otherwise has no disk side effects of its own — see
// internal/tui/save.go). If the key was empty, the returned line has
// an empty value ("KEY="), which the caller/user still needs to fill
// in.
func (c *ComposeConfig) ConvertSecretToEnvFile(serviceName, key string) (envFileLine string, err error) {
	svc := c.GetService(serviceName)
	if svc == nil {
		return "", fmt.Errorf("service %q not found", serviceName)
	}
	idx, value, found := findEnvEntry(svc.Environment, key)
	if !found {
		return "", fmt.Errorf("service %q has no environment entry %q", serviceName, key)
	}
	svc.Environment[idx] = fmt.Sprintf("%s=${%s}", key, key)
	return fmt.Sprintf("%s=%s", key, value), nil
}

// ConvertSecretToComposeSecret converts a hardcoded (or empty) secret
// environment entry into a Compose "secrets:" reference instead: it
// removes the environment entry, adds a top-level file-backed secret
// (merging into any existing "secrets:" block rather than replacing
// it) and a matching "secrets:" reference on the service, and returns
// the relative file path the caller should write value into plus the
// value itself.
//
// This is a bigger behavior change than the .env conversion: Compose
// secrets are delivered to the container as a file under
// /run/secrets/<name>, not as an environment variable — the
// application has to read it from there. It's the pattern Compose
// itself recommends for genuinely sensitive values, but unlike the
// .env conversion it isn't a transparent drop-in replacement, so it's
// worth surfacing that distinction to whoever is choosing between the
// two (see the three-way "keep as is / .env / Compose secret" choice
// this backs).
func (c *ComposeConfig) ConvertSecretToComposeSecret(serviceName, key string) (secretName, filePath, value string, err error) {
	svc := c.GetService(serviceName)
	if svc == nil {
		return "", "", "", fmt.Errorf("service %q not found", serviceName)
	}
	idx, val, found := findEnvEntry(svc.Environment, key)
	if !found {
		return "", "", "", fmt.Errorf("service %q has no environment entry %q", serviceName, key)
	}

	secretName = strings.ToLower(serviceName) + "_" + strings.ToLower(key)
	filePath = "./secrets/" + secretName + ".txt"

	if err := mergeTopLevelSecret(c, secretName, filePath); err != nil {
		return "", "", "", err
	}
	if err := mergeServiceSecretRef(svc, secretName); err != nil {
		return "", "", "", err
	}

	svc.Environment = append(svc.Environment[:idx], svc.Environment[idx+1:]...)
	return secretName, filePath, val, nil
}

// mergeTopLevelSecret adds a file-backed secret definition to the
// config's top-level "secrets:" block, creating that block if it
// doesn't exist yet and merging into it (rather than overwriting) if
// it was already present — e.g. from an imported file, or an earlier
// call to this function for a different key. Returns an error instead
// of guessing if "secrets:" exists but isn't shaped as a mapping,
// since that would mean the existing file is already unusual enough
// that silently rewriting it risks corrupting it.
func mergeTopLevelSecret(c *ComposeConfig, secretName, filePath string) error {
	entry, err := toNode(map[string]interface{}{"file": filePath})
	if err != nil {
		return err
	}

	existing, ok := c.Extra["secrets"]
	if !ok {
		mapping := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		appendKV(mapping, secretName, entry)
		if c.Extra == nil {
			c.Extra = map[string]yaml.Node{}
		}
		c.Extra["secrets"] = *mapping
		return nil
	}

	if existing.Kind != yaml.MappingNode {
		return fmt.Errorf("top-level 'secrets:' is not a mapping, refusing to merge")
	}
	if secretsMappingHasKey(&existing, secretName) {
		// Already defined (e.g. converting a second env var that
		// happens to produce the same name) — leave it as-is rather
		// than adding a duplicate key.
		return nil
	}
	appendKV(&existing, secretName, entry)
	c.Extra["secrets"] = existing
	return nil
}

// mergeServiceSecretRef adds secretName to the service's "secrets:"
// list, creating it if needed and skipping the add if it's already
// referenced.
func mergeServiceSecretRef(svc *ServiceConfig, secretName string) error {
	existing, ok := svc.Extra["secrets"]
	if !ok {
		seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		seq.Content = append(seq.Content, scalar(secretName))
		if svc.Extra == nil {
			svc.Extra = map[string]yaml.Node{}
		}
		svc.Extra["secrets"] = *seq
		return nil
	}

	if existing.Kind != yaml.SequenceNode {
		return fmt.Errorf("service 'secrets:' is not a list, refusing to merge")
	}
	for _, item := range existing.Content {
		if item.Value == secretName {
			return nil
		}
	}
	existing.Content = append(existing.Content, scalar(secretName))
	svc.Extra["secrets"] = existing
	return nil
}

// secretsMappingHasKey reports whether a "secrets:" mapping node
// already defines the given key.
func secretsMappingHasKey(mapping *yaml.Node, key string) bool {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return true
		}
	}
	return false
}
