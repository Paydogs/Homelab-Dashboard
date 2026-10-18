package domain

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// The dashboard content is a tree: Group nodes are the parents — each one
// can hold other Groups (sub-panels) and Child nodes. Children are the leaf
// ends of the tree and never hold anything. There are two child kinds:
// Service (a link opened in the browser) and App (a locally runnable
// executable).

// Leaf carries the fields shared by every child.
type Leaf struct {
	Title    string   `yaml:"title"`
	Subtitle string   `yaml:"subtitle,omitempty"`
	URL      string   `yaml:"url,omitempty"` // opened in a browser tab
	Icon     string   `yaml:"icon,omitempty"`
	Tags     []string `yaml:"tags,omitempty"`
	Status   Status   `yaml:"-"` // runtime state, merged in at render time
}

// Child is one end leaf of the tree (a Service or an App).
type Child interface {
	// isChild marks implementors; both kinds embed Leaf.
	isChild()

	// Core exposes the shared leaf fields (named Core to avoid clashing
	// with the promoted `Leaf` field).
	Core() *Leaf
}

// Service is a child that links to a remote service (usually another VM).
type Service struct {
	Leaf   `yaml:",inline"`
	Health *HealthCheck `yaml:"health,omitempty"`
}

func (*Service) isChild() {}

func (s *Service) Core() *Leaf { return &s.Leaf }

// App is a child that launches a locally installed executable on the
// dashboard's host machine. It is offered through a confirm step and only
// meaningful when the dashboard runs on that machine.
type App struct {
	Leaf `yaml:",inline"`
	Run  string `yaml:"run"`
}

func (*App) isChild() {}

func (a *App) Core() *Leaf { return &a.Leaf }

// IsRunnable reports whether the child is a locally runnable app.
func IsRunnable(c Child) bool {
	_, ok := c.(*App)
	return ok
}

// Group is a parent node: a titled panel holding sub-groups and children.
type Group struct {
	Title    string    `yaml:"title"`
	Subtitle string    `yaml:"subtitle,omitempty"`
	Icon     string    `yaml:"icon,omitempty"`
	Host     string    `yaml:"host,omitempty"`
	Groups   []Group   `yaml:"groups,omitempty"`
	Children ChildList `yaml:"items,omitempty"`
}

// Config is the root of the YAML config file — the source of truth.
type Config struct {
	Title  string  `yaml:"title"`
	Groups []Group `yaml:"groups"`

	// DefaultStyle is the style (resources/styles folder name, e.g.
	// "modern") selected on first visit; a viewer's own pick is kept in
	// the browser. When unset or not found, the first style folder wins.
	DefaultStyle string `yaml:"defaultStyle,omitempty"`

	// EditAccess controls who may use the web editors (config + style):
	// "local" (default) — only the dashboard's own machine; "lan" — any
	// private-network client; "any" — no restriction (public deploys
	// beware). The local `run:` launcher stays local-only regardless.
	EditAccess string `yaml:"editAccess,omitempty"`

	// Editable is the simple switch for enabling editing from wherever the
	// dashboard is browsed (a VM serving the LAN included): true behaves
	// like editAccess "lan". An explicitly set editAccess wins over it.
	Editable bool `yaml:"editable,omitempty"`

	// EditAccessSet records whether editAccess was spelled out in the YAML
	// (the explicit option then takes priority over Editable). Not in YAML.
	EditAccessSet bool `yaml:"-"`
}

// Walk visits every group in the tree, in order, with its full path
// (titles joined with "/", e.g. "Servers/Server 1").
func (c *Config) Walk(fn func(path string, g *Group)) {
	var walk func(path string, g *Group)
	walk = func(path string, g *Group) {
		p := g.Title
		if path != "" {
			p = path + "/" + g.Title
		}
		fn(p, g)
		for i := range g.Groups {
			walk(p, &g.Groups[i])
		}
	}
	for i := range c.Groups {
		walk("", &c.Groups[i])
	}
}

// ChildPath joins a group path and a child title into the child's unique
// key (also its URL path segment on the API).
func ChildPath(groupPath, title string) string {
	return groupPath + "/" + title
}

// FindService resolves a child path key (as built by ChildPath) back to its
// group and child.
func (c *Config) FindService(key string) (*Group, Child, bool) {
	var found *Group
	var child Child
	c.Walk(func(path string, g *Group) {
		for _, ch := range g.Children {
			if ChildPath(path, ch.Core().Title) == key {
				found, child = g, ch
			}
		}
	})
	return found, child, child != nil
}

// AllTags returns the distinct tags across the whole tree, sorted.
func (c *Config) AllTags() []string {
	set := map[string]bool{}
	c.Walk(func(path string, g *Group) {
		for _, ch := range g.Children {
			for _, t := range ch.Core().Tags {
				set[t] = true
			}
		}
	})
	out := make([]string, 0, len(set))
	for t := range set {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// ChildList decodes the `items` list, discriminating each entry into a
// Service (has a url) or an App (has a run path).
type ChildList []Child

func (l *ChildList) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.SequenceNode {
		return fmt.Errorf("items must be a list")
	}
	items := make([]Child, 0, len(value.Content))
	for _, itemNode := range value.Content {
		var probe struct {
			Leaf   `yaml:",inline"`
			Run    string       `yaml:"run,omitempty"`
			Health *HealthCheck `yaml:"health,omitempty"`
		}
		if err := decodeNodeStrict(itemNode, &probe); err != nil {
			return err
		}
		if probe.Title == "" {
			return fmt.Errorf("every item needs a title")
		}
		switch {
		case probe.URL != "" && probe.Run != "":
			return fmt.Errorf("item %q: set exactly one of `url` or `run`", probe.Title)
		case probe.Run != "" && probe.Health != nil:
			return fmt.Errorf("item %q: healthchecks apply to `url` items only", probe.Title)
		case probe.URL != "":
			items = append(items, &Service{Leaf: probe.Leaf, Health: probe.Health})
		case probe.Run != "":
			items = append(items, &App{Leaf: probe.Leaf, Run: probe.Run})
		default:
			return fmt.Errorf("item %q: set exactly one of `url` or `run`", probe.Title)
		}
	}
	*l = items
	return nil
}

// decodeNodeStrict decodes a single YAML node with unknown-field
// rejection — the strictness the top-level decoder enforces must be applied
// by hand inside custom unmarshallers.
func decodeNodeStrict(node *yaml.Node, out any) error {
	data, err := yaml.Marshal(node)
	if err != nil {
		return err
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("%s", strings.TrimSpace(err.Error()))
	}
	return nil
}
