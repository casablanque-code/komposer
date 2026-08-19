package composer

import (
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

	doc := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}}
	return yaml.Marshal(doc)
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

	if len(svc.DependsOn) == 0 {
		return base, nil
	}

	depNode := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for _, dep := range svc.DependsOn {
		condNode := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		appendKV(condNode, "condition", scalar(string(dep.Condition)))
		appendKV(depNode, dep.Service, condNode)
	}
	appendKV(base, "depends_on", depNode)

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
