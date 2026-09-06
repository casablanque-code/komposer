package composer

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v5"
	"gopkg.in/yaml.v3"
)

// composeSchemaFS embeds the official Compose Specification JSON
// Schema, vendored from github.com/compose-spec/compose-spec
// (schema/compose-spec.json). It's checked in as a plain file rather
// than fetched at runtime so validation works offline and
// deterministically — update schema/compose-spec.json from upstream
// when a new Compose Spec revision ships.
//
//go:embed schema/compose-spec.json
var composeSchemaFS embed.FS

var composeSpecSchema *jsonschema.Schema

func init() {
	schemaBytes, err := composeSchemaFS.ReadFile("schema/compose-spec.json")
	if err != nil {
		panic(fmt.Sprintf("composer: embedded compose-spec.json missing: %v", err))
	}

	c := jsonschema.NewCompiler()
	c.Draft = jsonschema.Draft2020
	if err := c.AddResource("compose-spec.json", bytes.NewReader(schemaBytes)); err != nil {
		panic(fmt.Sprintf("composer: failed to load compose-spec.json: %v", err))
	}

	composeSpecSchema, err = c.Compile("compose-spec.json")
	if err != nil {
		panic(fmt.Sprintf("composer: failed to compile compose-spec.json: %v", err))
	}
}

// SpecValidationResult is the outcome of checking a Compose YAML
// document against the official Compose Specification schema. This is
// deliberately a separate, narrower type from ValidationResult:
// ValidationResult carries komposer's own opinionated checks (required
// fields, port syntax, depends_on references, security advisories —
// see Validate()), which are useful but are not the Compose Spec.
// SpecValidationResult only answers "does Docker Compose itself
// recognize this document's shape at all".
type SpecValidationResult struct {
	Valid  bool
	Issues []string
}

// ValidateAgainstSpec renders the current config to YAML and checks it
// against the official Compose Specification schema.
func (c *ComposeConfig) ValidateAgainstSpec() (SpecValidationResult, error) {
	yamlBytes, err := c.ExportYAML()
	if err != nil {
		return SpecValidationResult{}, fmt.Errorf("rendering YAML for spec validation: %w", err)
	}
	return ValidateYAMLAgainstSpec(yamlBytes)
}

// ValidateYAMLAgainstSpec checks an arbitrary Compose YAML document
// (e.g. one loaded from disk via ImportYAML, before or instead of
// converting it into a ComposeConfig) against the official schema.
func ValidateYAMLAgainstSpec(yamlBytes []byte) (SpecValidationResult, error) {
	var raw interface{}
	if err := yaml.Unmarshal(yamlBytes, &raw); err != nil {
		return SpecValidationResult{}, fmt.Errorf("parsing YAML: %w", err)
	}

	// jsonschema validates data shaped the way encoding/json decodes
	// it (numbers as float64, maps as map[string]interface{}).
	// yaml.v3 decodes integers as int and can nest other Go-native
	// types the validator doesn't expect, so round-trip through JSON
	// to normalize before validating.
	normalized, err := jsonRoundTrip(raw)
	if err != nil {
		return SpecValidationResult{}, fmt.Errorf("normalizing document: %w", err)
	}

	if err := composeSpecSchema.Validate(normalized); err != nil {
		if validationErr, ok := err.(*jsonschema.ValidationError); ok {
			return SpecValidationResult{Valid: false, Issues: flattenSchemaErrors(validationErr)}, nil
		}
		// Not a *jsonschema.ValidationError — some other failure
		// (shouldn't normally happen with a precompiled schema).
		// Surface it as a single issue rather than swallowing it.
		return SpecValidationResult{Valid: false, Issues: []string{err.Error()}}, nil
	}

	return SpecValidationResult{Valid: true}, nil
}

func jsonRoundTrip(v interface{}) (interface{}, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out interface{}
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// flattenSchemaErrors walks a jsonschema.ValidationError tree — which
// nests one branch per failed alternative of keywords like oneOf/anyOf
// — into a flat, deduplicated list of human-readable messages, each
// prefixed with the JSON pointer path of the offending field. Only
// leaf causes carry an actionable message; intermediate nodes just say
// "doesn't validate with <schema>", which isn't useful to show anyone.
func flattenSchemaErrors(err *jsonschema.ValidationError) []string {
	var out []string
	seen := make(map[string]bool)

	var walk func(e *jsonschema.ValidationError)
	walk = func(e *jsonschema.ValidationError) {
		if e == nil {
			return
		}
		if len(e.Causes) == 0 {
			loc := e.InstanceLocation
			if loc == "" {
				loc = "/"
			}
			msg := fmt.Sprintf("%s: %s", loc, e.Message)
			if !seen[msg] {
				seen[msg] = true
				out = append(out, msg)
			}
			return
		}
		for _, cause := range e.Causes {
			walk(cause)
		}
	}
	walk(err)

	if len(out) == 0 {
		out = append(out, err.Error())
	}
	return out
}
