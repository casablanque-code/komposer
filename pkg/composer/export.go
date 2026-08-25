package composer

import (
	"bytes"
	"fmt"

	"gopkg.in/yaml.v3"
)

// ExportYAML renders the current configuration as a docker-compose.yml
// byte slice. Service and depends_on ordering follows the order in which
// the user added them in the UI (see ComposeConfig.Services), not
// alphabetical order, since plain Go maps would otherwise force yaml.v3
// to sort keys.
func (c *ComposeConfig) ExportYAML() ([]byte, error) {
	root := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}

	if c.Version != "" {
		appendKV(root, "version", scalar(c.Version))
	}

	servicesNode := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for _, entry := range c.Services {
		svcNode, err := serviceToNode(entry.Config)
		if err != nil {
			return nil, fmt.Errorf("service %q: %w", entry.Name, err)
		}
		appendKV(servicesNode, entry.Name, svcNode)
	}
	appendKV(root, "services", servicesNode)

	if len(c.Volumes) > 0 {
		node, err := toNode(c.Volumes)
		if err != nil {
			return nil, err
		}
		appendKV(root, "volumes", node)
	}

	if len(c.Networks) > 0 {
		node, err := toNode(c.Networks)
		if err != nil {
			return nil, err
		}
		appendKV(root, "networks", node)
	}

	// Re-emit any top-level fields this tool doesn't model explicitly
	// (secrets:, configs:, name:, x-* extensions, ...) exactly as they
	// came in on import — see ComposeConfig.Extra's doc comment. Map
	// iteration order is random in Go, so these come out in whatever
	// order happens to occur this run — acceptable, since the
	// alternative (losing them outright) is far worse than not
	// preserving their exact original position among each other.
	for key, node := range c.Extra {
		node := node
		appendKV(root, key, &node)
	}

	doc := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}}

	// yaml.Marshal's package-level helper defaults to a 4-space indent,
	// which is valid YAML but not what any real-world docker-compose.yml
	// uses — every reference file, the Compose spec's own examples, and
	// every other tool in this ecosystem indent with 2 spaces. Going
	// through an explicit Encoder with SetIndent(2) is the only way to
	// control this; the package-level Marshal function hardcodes 4 and
	// exposes no option to change it.
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// serviceToNode converts a ServiceConfig into a yaml.Node, handling the
// ordered depends_on map manually and delegating the rest to the standard
// struct tags via an intermediate marshal/remarshal.
func serviceToNode(svc *ServiceConfig) (*yaml.Node, error) {
	// Marshal the struct fields that use standard yaml tags first.
	base, err := toNode(svc)
	if err != nil {
		return nil, err
	}

	if len(svc.DependsOn) > 0 {
		depNode := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		for _, dep := range svc.DependsOn {
			condNode := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			appendKV(condNode, "condition", scalar(string(dep.Condition)))
			appendKV(depNode, dep.Service, condNode)
		}
		appendKV(base, "depends_on", depNode)
	}

	// Re-emit any service-level fields this tool doesn't model
	// explicitly (command, networks, container_name, labels, env_file,
	// ...) exactly as they came in on import — see
	// ServiceConfig.Extra's doc comment. Without this, importing a real
	// docker-compose.yml and saving it back through the TUI would
	// silently drop every field the form doesn't know about, even ones
	// the user never touched.
	for key, node := range svc.Extra {
		node := node
		appendKV(base, key, &node)
	}

	return base, nil
}

func toNode(v interface{}) (*yaml.Node, error) {
	var node yaml.Node
	if err := node.Encode(v); err != nil {
		return nil, err
	}
	return &node, nil
}

func scalar(s string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s}
}

func appendKV(m *yaml.Node, key string, value *yaml.Node) {
	m.Content = append(m.Content, scalar(key), value)
}
