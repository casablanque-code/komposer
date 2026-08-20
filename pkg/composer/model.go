// Package composer holds the internal data model used to represent a
// docker-compose configuration while it is being edited in the TUI, and the
// logic to marshal that model into a docker-compose.yml file.
package composer

// ComposeConfig represents the root Docker Compose structure.
//
// Services is kept as an ordered slice (ServiceEntry) rather than a bare
// map so that the order in which the user added services in the UI is
// preserved when exporting YAML. Use the helper methods below to interact
// with services instead of manipulating the slice directly.
type ComposeConfig struct {
	// Version is intentionally omitted from output by default: the
	// top-level `version:` key is deprecated by the Compose Specification.
	// It is kept here only for compatibility with older tooling if a user
	// explicitly sets it.
	Version  string                 `yaml:"version,omitempty"`
	Services []ServiceEntry         `yaml:"-"`
	Volumes  map[string]interface{} `yaml:"volumes,omitempty"`
	Networks map[string]interface{} `yaml:"networks,omitempty"`
}

// ServiceEntry pairs a service name with its configuration, preserving
// insertion order (unlike a plain Go map).
type ServiceEntry struct {
	Name   string
	Config *ServiceConfig
}

// ServiceConfig holds configuration for an individual container/service.
type ServiceConfig struct {
	Image       string           `yaml:"image,omitempty"`
	Build       string           `yaml:"build,omitempty"`
	Ports       []string         `yaml:"ports,omitempty"`
	Environment []string         `yaml:"environment,omitempty"`
	Volumes     []string         `yaml:"volumes,omitempty"`
	DependsOn   []DependsOnEntry `yaml:"-"`
	HealthCheck *HealthCheck     `yaml:"healthcheck,omitempty"`
	Restart     string           `yaml:"restart,omitempty"`
	// User and Privileged aren't editable from the TUI form yet, but
	// they still need to round-trip: without a struct field for them,
	// ImportYAML silently drops `user:`/`privileged:` from any file
	// that had them, and the validator has nothing to warn about. Kept
	// even though there's no form field so an imported file that sets
	// these doesn't lose them on the next export.
	User       string `yaml:"user,omitempty"`
	Privileged bool   `yaml:"privileged,omitempty"`
}

// DependsOnEntry pairs a dependency's service name with its wait condition,
// again preserving insertion order for stable, predictable YAML output.
type DependsOnEntry struct {
	Service   string
	Condition DependsOnCond
}

// DependsOnCond is the docker-compose depends_on wait condition.
type DependsOnCond string

const (
	CondServiceStarted               DependsOnCond = "service_started"
	CondServiceHealthy               DependsOnCond = "service_healthy"
	CondServiceCompletedSuccessfully DependsOnCond = "service_completed_successfully"
)

// HealthCheck maps to the docker-compose healthcheck block.
type HealthCheck struct {
	Test     []string `yaml:"test"`
	Interval string   `yaml:"interval,omitempty"`
	Timeout  string   `yaml:"timeout,omitempty"`
	Retries  int      `yaml:"retries,omitempty"`
}

// NewComposeConfig returns an empty, ready-to-use ComposeConfig.
func NewComposeConfig() *ComposeConfig {
	return &ComposeConfig{
		Services: []ServiceEntry{},
	}
}

// AddService appends a new service with the given name and an empty
// configuration, then returns the created ServiceConfig for further
// editing. If a service with the same name already exists, it is
// returned unchanged instead of creating a duplicate.
func (c *ComposeConfig) AddService(name string) *ServiceConfig {
	if existing := c.GetService(name); existing != nil {
		return existing
	}
	svc := &ServiceConfig{}
	c.Services = append(c.Services, ServiceEntry{Name: name, Config: svc})
	return svc
}

// GetService returns the ServiceConfig for the given name, or nil if no
// such service exists.
func (c *ComposeConfig) GetService(name string) *ServiceConfig {
	for _, e := range c.Services {
		if e.Name == name {
			return e.Config
		}
	}
	return nil
}

// RemoveService deletes the service with the given name, if present.
// Reports whether a service was actually removed.
func (c *ComposeConfig) RemoveService(name string) bool {
	for i, e := range c.Services {
		if e.Name == name {
			c.Services = append(c.Services[:i], c.Services[i+1:]...)
			return true
		}
	}
	return false
}

// RenameService renames a service in place, also fixing up any depends_on
// references to it from other services so the compose graph stays valid.
func (c *ComposeConfig) RenameService(oldName, newName string) bool {
	if oldName == newName {
		return true
	}
	if c.GetService(newName) != nil {
		return false // name collision
	}
	renamed := false
	for i, e := range c.Services {
		if e.Name == oldName {
			c.Services[i].Name = newName
			renamed = true
			break
		}
	}
	if !renamed {
		return false
	}
	for _, e := range c.Services {
		for i, dep := range e.Config.DependsOn {
			if dep.Service == oldName {
				e.Config.DependsOn[i].Service = newName
			}
		}
	}
	return true
}

// MoveService reorders the service at index i to index j (both must be
// valid indices into Services). Used for drag/reorder in the UI.
func (c *ComposeConfig) MoveService(i, j int) bool {
	if i < 0 || j < 0 || i >= len(c.Services) || j >= len(c.Services) || i == j {
		return false
	}
	e := c.Services[i]
	c.Services = append(c.Services[:i], c.Services[i+1:]...)
	if j > i {
		j--
	}
	c.Services = append(c.Services[:j], append([]ServiceEntry{e}, c.Services[j:]...)...)
	return true
}

// ServiceNames returns the names of all configured services, in order.
func (c *ComposeConfig) ServiceNames() []string {
	names := make([]string, 0, len(c.Services))
	for _, e := range c.Services {
		names = append(names, e.Name)
	}
	return names
}
