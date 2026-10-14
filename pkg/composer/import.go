package composer

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ImportYAML reads a docker-compose.yml file and parses it into a
// ComposeConfig.
//
// This works directly off the parsed yaml.Node tree rather than
// unmarshaling straight into ComposeConfig/ServiceConfig, specifically
// so nothing is lost: any top-level or service-level key this tool
// doesn't have an explicit field for is captured verbatim into the
// relevant Extra map (see ComposeConfig.Extra and
// ServiceConfig.Extra) instead of vanishing the moment the file is
// unmarshaled. Both structs have plenty of fields the real Compose
// spec supports that this tool's form can't edit — command, networks,
// container_name, labels, env_file, and many more — and a straight
// struct-tag unmarshal would silently drop every one of them from any
// real-world file that used them.
func ImportYAML(path string) (*ComposeConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	if len(root.Content) == 0 {
		return NewComposeConfig(), nil
	}
	doc := root.Content[0]
	if doc.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("expected a YAML mapping at the document root")
	}

	config := NewComposeConfig()

	for i := 0; i+1 < len(doc.Content); i += 2 {
		key := doc.Content[i].Value
		valueNode := doc.Content[i+1]

		switch key {
		case "version":
			config.Version = valueNode.Value

		case "services":
			services, err := parseServices(valueNode)
			if err != nil {
				return nil, err
			}
			config.Services = services

		case "volumes":
			var v map[string]interface{}
			if err := valueNode.Decode(&v); err != nil {
				return nil, fmt.Errorf("volumes: %w", err)
			}
			config.Volumes = v

		case "networks":
			var n map[string]interface{}
			if err := valueNode.Decode(&n); err != nil {
				return nil, fmt.Errorf("networks: %w", err)
			}
			config.Networks = n

		default:
			if config.Extra == nil {
				config.Extra = make(map[string]yaml.Node)
			}
			config.Extra[key] = *valueNode
		}
	}

	return config, nil
}

func parseServices(node *yaml.Node) ([]ServiceEntry, error) {
	if node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("services: expected a mapping")
	}

	var services []ServiceEntry
	for i := 0; i+1 < len(node.Content); i += 2 {
		name := node.Content[i].Value
		svcNode := node.Content[i+1]

		svc, err := parseService(svcNode)
		if err != nil {
			return nil, fmt.Errorf("service %q: %w", name, err)
		}
		services = append(services, ServiceEntry{Name: name, Config: svc})
	}
	return services, nil
}

func parseService(node *yaml.Node) (*ServiceConfig, error) {
	svc := &ServiceConfig{}
	if node.Kind != yaml.MappingNode {
		return svc, nil
	}

	// Every known key below is decoded individually, on its own node,
	// instead of the previous single node.Decode(svc) covering every
	// standard-tagged field in one pass. That one-shot decode assumed
	// every real-world file would use exactly the shape each Go field
	// declares — e.g. `ports`/`volumes` as a plain string list and
	// `environment` as a list, never a mapping — which the Compose
	// spec does not actually guarantee: `environment: {KEY: value}`
	// (mapping form), long-syntax `ports`/`volumes` entries (mappings
	// with target/published/source keys), and a mapping-form `build`
	// are all valid and common. Any one of them being present made
	// node.Decode(svc) fail with a type-mismatch error that aborted
	// the *entire* import — turning "this tool doesn't have a field
	// for that shape" into "this file won't open at all". Decoding
	// key-by-key means a shape mismatch on one field can fall back to
	// Extra (preserved verbatim, same as any other unmodeled key)
	// without taking the rest of a perfectly valid file down with it.
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i].Value
		valueNode := node.Content[i+1]

		switch key {
		case "image":
			svc.Image = valueNode.Value

		case "build":
			// Short form only (`build: ./dir`); the long mapping form
			// (`build: {context: ..., dockerfile: ...}`) has no form
			// field, so it round-trips through Extra instead.
			if valueNode.Kind == yaml.ScalarNode {
				svc.Build = valueNode.Value
			} else {
				setExtra(svc, "build", valueNode)
			}

		case "ports", "volumes":
			// Short-syntax entries (plain strings) go into the modeled
			// field the form edits; long-syntax entries (mappings with
			// target/published/source/...) are kept verbatim in Extra
			// as a sequence of their own. ExportYAML merges the two
			// back into one list, so a list mixing both syntaxes — or
			// a long-syntax list the user then adds a short entry to
			// through the form — survives instead of producing a
			// duplicate key or losing entries.
			shorts, longs, ok := splitMixedList(valueNode)
			switch {
			case !ok:
				setExtra(svc, key, valueNode)
			default:
				if key == "ports" {
					svc.Ports = shorts
				} else {
					svc.Volumes = shorts
				}
				if len(longs) > 0 {
					setExtra(svc, key, &yaml.Node{
						Kind:    yaml.SequenceNode,
						Tag:     "!!seq",
						Content: longs,
					})
				}
			}

		case "environment":
			env, err := parseEnvironment(valueNode)
			if err != nil {
				return nil, fmt.Errorf("environment: %w", err)
			}
			svc.Environment = env

		case "depends_on":
			deps, err := parseDependsOn(valueNode)
			if err != nil {
				return nil, fmt.Errorf("depends_on: %w", err)
			}
			svc.DependsOn = deps

		case "healthcheck":
			var hc HealthCheck
			if isModeledHealthcheck(valueNode) && valueNode.Decode(&hc) == nil {
				svc.HealthCheck = &hc
			} else {
				// Anything the form can't represent round-trips
				// through Extra instead of being narrowed: a `test:`
				// given as the plain-string shorthand, `disable:
				// true`, a block with no `test` (which only tunes
				// the image's own HEALTHCHECK), or any key
				// HealthCheck has no field for.
				setExtra(svc, "healthcheck", valueNode)
			}

		case "restart":
			svc.Restart = valueNode.Value

		case "user":
			svc.User = valueNode.Value

		case "privileged":
			var b bool
			if valueNode.Decode(&b) == nil {
				svc.Privileged = b
			}

		default:
			setExtra(svc, key, valueNode)
		}
	}

	return svc, nil
}

// healthcheckKeys are the healthcheck keys HealthCheck has fields for.
var healthcheckKeys = map[string]bool{
	"test": true, "interval": true, "timeout": true, "retries": true,
	"start_period": true, "start_interval": true,
}

// isModeledHealthcheck reports whether a healthcheck block can be held in
// HealthCheck without losing anything: a mapping with a list-form `test`
// and no keys beyond the ones HealthCheck has fields for. (Decoding a
// block with other keys into HealthCheck would silently drop them.)
func isModeledHealthcheck(node *yaml.Node) bool {
	if node.Kind != yaml.MappingNode {
		return false
	}
	test := keyValue(node, "test")
	if test == nil || test.Kind != yaml.SequenceNode {
		return false
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if !healthcheckKeys[node.Content[i].Value] {
			return false
		}
	}
	return true
}

// splitMixedList splits a ports/volumes sequence into its short-syntax
// scalar entries and its long-syntax mapping entries. Reports false for
// anything else (not a sequence, or items that are neither scalars nor
// mappings, such as aliases), so the caller can keep the whole node
// verbatim instead.
func splitMixedList(node *yaml.Node) (shorts []string, longs []*yaml.Node, ok bool) {
	if node.Kind != yaml.SequenceNode {
		return nil, nil, false
	}
	for _, item := range node.Content {
		switch item.Kind {
		case yaml.ScalarNode:
			shorts = append(shorts, item.Value)
		case yaml.MappingNode:
			longs = append(longs, item)
		default:
			return nil, nil, false
		}
	}
	return shorts, longs, true
}

// decodeScalarList decodes a YAML sequence of plain scalars (the
// docker-compose "short syntax" used by ports/volumes) into a []string.
// Reports false — without error — for anything else, so the caller can
// fall back to preserving the node verbatim instead of failing the
// import outright; see parseService.
func decodeScalarList(node *yaml.Node) ([]string, bool) {
	if node.Kind != yaml.SequenceNode {
		return nil, false
	}
	out := make([]string, 0, len(node.Content))
	for _, item := range node.Content {
		if item.Kind != yaml.ScalarNode {
			return nil, false
		}
		out = append(out, item.Value)
	}
	return out, true
}

// setExtra records a raw, unmodeled service-level key so it survives
// round-tripping through ExportYAML untouched.
func setExtra(svc *ServiceConfig, key string, node *yaml.Node) {
	if svc.Extra == nil {
		svc.Extra = make(map[string]yaml.Node)
	}
	svc.Extra[key] = *node
}

// parseEnvironment supports both docker-compose `environment` forms:
// the list form ("environment: [FOO=bar]") and the mapping form
// ("environment: {FOO: bar}"), normalizing the latter into the same
// "KEY=value" list form so it's editable through the same form field
// either way. A mapping value of null (docker-compose's shorthand for
// "pass this variable through from the host environment", e.g.
// `environment: {FOO:}`) becomes a bare "FOO" entry, matching how
// docker compose itself treats that shorthand.
//
// Previously `environment` was decoded as a plain []string via the
// blanket struct decode in parseService, which errored out — aborting
// the whole import — the moment it hit the mapping form, even though
// the mapping form is equally valid Compose YAML and arguably more
// common than the list form in hand-written files.
func parseEnvironment(node *yaml.Node) ([]string, error) {
	switch node.Kind {
	case yaml.SequenceNode:
		var list []string
		if err := node.Decode(&list); err != nil {
			return nil, err
		}
		return list, nil

	case yaml.MappingNode:
		var out []string
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := node.Content[i].Value
			val := node.Content[i+1]
			switch {
			case val.Kind != yaml.ScalarNode:
				return nil, fmt.Errorf("%s: expected a scalar value", key)
			case val.Tag == "!!null":
				// `FOO:` / `FOO: null` / `FOO: ~` — pass through from
				// the host environment.
				out = append(out, key)
			default:
				// Includes `FOO: ""`, which sets the variable to an
				// empty string — not the same thing as passing it
				// through, so it must come out as "FOO=".
				out = append(out, key+"="+val.Value)
			}
		}
		return out, nil

	default:
		return nil, fmt.Errorf("expected a list or mapping")
	}
}

// parseDependsOn supports both docker-compose depends_on forms: the
// short list form ("depends_on: [db, cache]", condition defaults to
// service_started) and the long map form with explicit conditions
// ("depends_on: {db: {condition: service_healthy}}").
//
// This didn't exist before at all: ServiceConfig.DependsOn is tagged
// yaml:"-" so the standard struct-tag decode step in parseService
// always skips it, and nothing was filling it in afterward — meaning
// depends_on was silently dropped from every imported file the moment
// it was parsed, before the user even got a chance to see or edit it.
// Saving back over the original file would then permanently remove
// those dependencies.
func parseDependsOn(node *yaml.Node) ([]DependsOnEntry, error) {
	switch node.Kind {
	case yaml.SequenceNode:
		var names []string
		if err := node.Decode(&names); err != nil {
			return nil, err
		}
		deps := make([]DependsOnEntry, len(names))
		for i, n := range names {
			deps[i] = DependsOnEntry{Service: n, Condition: CondServiceStarted}
		}
		return deps, nil

	case yaml.MappingNode:
		var deps []DependsOnEntry
		for i := 0; i+1 < len(node.Content); i += 2 {
			name := node.Content[i].Value
			var detail struct {
				Condition string `yaml:"condition"`
			}
			if err := node.Content[i+1].Decode(&detail); err != nil {
				return nil, err
			}
			cond := DependsOnCond(detail.Condition)
			if cond == "" {
				cond = CondServiceStarted
			}
			deps = append(deps, DependsOnEntry{Service: name, Condition: cond})
		}
		return deps, nil

	default:
		return nil, fmt.Errorf("expected a list or mapping")
	}
}
