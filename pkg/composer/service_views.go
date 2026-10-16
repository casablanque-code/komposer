package composer

import (
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// This file holds read-only views over a ServiceConfig that look at both
// the modeled fields (what the TUI form edits) and ServiceConfig.Extra
// (what ImportYAML preserved verbatim because the form has no field for
// that shape). Validation has to consult both: a service whose build is
// a mapping, whose healthcheck uses `disable:` or whose volumes use the
// long syntax is not "missing" any of them.

func (s *ServiceConfig) hasExtra(key string) bool {
	_, ok := s.Extra[key]
	return ok
}

// HasBuild reports whether the service defines a build, in either the
// short form (`build: ./dir`) or the mapping form (`build: {context: ...}`).
func (s *ServiceConfig) HasBuild() bool {
	return s.Build != "" || s.hasExtra("build")
}

// HasHealthCheck reports whether the service defines a healthcheck, whether
// or not it is one the form can edit (string-form test, `disable: true`,
// a block without `test` that only tunes the image's own HEALTHCHECK, ...).
func (s *ServiceConfig) HasHealthCheck() bool {
	return s.HealthCheck != nil || s.hasExtra("healthcheck")
}

// HasVolumes reports whether the service mounts anything, in short or
// long syntax.
func (s *ServiceConfig) HasVolumes() bool {
	return len(s.Volumes) > 0 || s.hasExtra("volumes")
}

// longFormEntries returns the long-syntax (mapping) entries kept in
// Extra[key] as string maps. Only scalar values are included; that is all
// the checks need.
func (s *ServiceConfig) longFormEntries(key string) []map[string]string {
	node, ok := s.Extra[key]
	if !ok || node.Kind != yaml.SequenceNode {
		return nil
	}
	var out []map[string]string
	for _, item := range node.Content {
		if item.Kind != yaml.MappingNode {
			continue
		}
		entry := make(map[string]string)
		for i := 0; i+1 < len(item.Content); i += 2 {
			if v := item.Content[i+1]; v.Kind == yaml.ScalarNode {
				entry[item.Content[i].Value] = v.Value
			}
		}
		out = append(out, entry)
	}
	return out
}

// PortStrings returns every published port as a short-syntax string: the
// entries of Ports as written, followed by long-syntax entries rewritten
// to the equivalent short form. The result is for analysis only (it is
// never exported), so checks written against the short syntax also cover
// `ports: [{target: 80, published: 8080, host_ip: 0.0.0.0}]`.
func (s *ServiceConfig) PortStrings() []string {
	out := append([]string(nil), s.Ports...)
	for _, p := range s.longFormEntries("ports") {
		target := p["target"]
		if target == "" {
			continue
		}
		short := target
		switch {
		case p["host_ip"] != "":
			ip := p["host_ip"]
			if strings.Contains(ip, ":") { // IPv6 needs brackets in short syntax
				ip = "[" + ip + "]"
			}
			short = ip + ":" + p["published"] + ":" + target
		case p["published"] != "":
			short = p["published"] + ":" + target
		}
		if proto := p["protocol"]; proto != "" && proto != "tcp" {
			short += "/" + proto
		}
		out = append(out, short)
	}
	return out
}

// VolumeStrings returns every mount as a string to search: short-syntax
// entries as written, and for long-syntax entries the source and the
// target (the places a host path such as the Docker socket can appear).
func (s *ServiceConfig) VolumeStrings() []string {
	out := append([]string(nil), s.Volumes...)
	for _, v := range s.longFormEntries("volumes") {
		if src := v["source"]; src != "" {
			out = append(out, src)
		}
		if tgt := v["target"]; tgt != "" {
			out = append(out, tgt)
		}
	}
	return out
}

// sortedExtraKeys returns the keys of an Extra map in a stable order, so
// the exported YAML is identical from run to run (Go randomizes map
// iteration, which would otherwise shuffle unmodeled keys on every save
// and show up as noise in version control).
func sortedExtraKeys(m map[string]yaml.Node) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// keyValue returns the value node stored under key in a mapping node, or
// nil if the mapping has no such key.
func keyValue(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// mountSpec is a volume mount reduced to what the security checks need.
type mountSpec struct {
	Source   string
	Target   string
	ReadOnly bool
}

// Mounts returns every mount of the service, short and long syntax alike.
// Only bind-style mounts are meaningful to the checks: for short syntax
// the source is whatever precedes the first colon, so a named volume
// ("pgdata:/var/lib/...") has a source that is not an absolute path.
func (s *ServiceConfig) Mounts() []mountSpec {
	var out []mountSpec
	for _, v := range s.Volumes {
		parts := strings.Split(v, ":")
		if len(parts) < 2 {
			continue
		}
		m := mountSpec{Source: parts[0], Target: parts[1]}
		if len(parts) >= 3 {
			for _, opt := range strings.Split(parts[2], ",") {
				if opt == "ro" {
					m.ReadOnly = true
				}
			}
		}
		out = append(out, m)
	}
	for _, v := range s.longFormEntries("volumes") {
		if t := v["type"]; t != "" && t != "bind" {
			continue
		}
		out = append(out, mountSpec{
			Source:   v["source"],
			Target:   v["target"],
			ReadOnly: v["read_only"] == "true",
		})
	}
	return out
}
