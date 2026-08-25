package composer

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// knownServiceKeys are the YAML keys ServiceConfig has an explicit,
// form-editable field for. Anything else found on a service during
// import goes into ServiceConfig.Extra instead of being silently
// dropped.
var knownServiceKeys = map[string]bool{
	"image": true, "build": true, "ports": true, "environment": true,
	"volumes": true, "depends_on": true, "healthcheck": true,
	"restart": true, "user": true, "privileged": true,
}

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

	// Populates every field with a standard yaml tag (image, build,
	// ports, environment, volumes, healthcheck, restart, user,
	// privileged) in one pass. depends_on is tagged yaml:"-" on the
	// struct — see parseDependsOn below for why it needs separate
	// handling — so this step correctly leaves it alone.
	if err := node.Decode(svc); err != nil {
		return nil, err
	}

	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i].Value
		valueNode := node.Content[i+1]

		switch {
		case key == "depends_on":
			deps, err := parseDependsOn(valueNode)
			if err != nil {
				return nil, fmt.Errorf("depends_on: %w", err)
			}
			svc.DependsOn = deps

		case knownServiceKeys[key]:
			// Already populated by node.Decode(svc) above.

		default:
			if svc.Extra == nil {
				svc.Extra = make(map[string]yaml.Node)
			}
			svc.Extra[key] = *valueNode
		}
	}

	return svc, nil
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
