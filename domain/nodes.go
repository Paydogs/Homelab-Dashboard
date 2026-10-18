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

// Group is a parent node: a titled container holding sub-groups and
// children. Groups are purely structural — a name, an optional subtitle,
// and the child/group lists; nothing else. In YAML the start line carries
// the name (`- group: Favourites`), so what a block is stays visible at a
// glance; `- title:` still works as an alias.
type Group struct {
	Title    string    `yaml:"-"`
	Subtitle string    `yaml:"subtitle,omitempty"`
	Groups   []Group   `yaml:"groups,omitempty"`
	Items    ChildList `yaml:"children,omitempty"`
}

// UnmarshalYAML accepts the self-explaining keys (group:, children:) and
// the plain ones (title:, items:) — mixing the pair in one block is an
// error.
func (g *Group) UnmarshalYAML(value *yaml.Node) error {
	var probe struct {
		Group    string    `yaml:"group"`
		Title    string    `yaml:"title"`
		Subtitle string    `yaml:"subtitle,omitempty"`
		Groups   []Group   `yaml:"groups,omitempty"`
		Children ChildList `yaml:"children,omitempty"`
		Items    ChildList `yaml:"items,omitempty"`
	}
	if err := decodeNodeStrict(value, &probe); err != nil {
		return err
	}
	name := probe.Group
	if probe.Title != "" {
		if name != "" {
			return fmt.Errorf("group: set either `group: %s` or `title:` — not both", probe.Group)
		}
		name = probe.Title
	}
	if name == "" {
		return fmt.Errorf("every group needs a name: start it with `- group: <name>`")
	}
	children, items := probe.Children, probe.Items
	switch {
	case len(children) > 0 && len(items) > 0:
		return fmt.Errorf("group %q: use either `children:` or `items:` — not both", name)
	case len(items) > 0:
		children = items
	}
	g.Title, g.Subtitle, g.Groups, g.Items = name, probe.Subtitle, probe.Groups, children
	return nil
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
		for _, ch := range g.Items {
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
		for _, ch := range g.Items {
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

// ChildList decodes the children list, discriminating each entry by its
// START LINE: `- service: <name>` builds a Service, `- app: <name>` builds
// an App; the plain `- title: <name>` still works and discriminates between
// them by the url/run fields.
type ChildList []Child

func (l *ChildList) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.SequenceNode {
		return fmt.Errorf("children must be a list")
	}
	items := make([]Child, 0, len(value.Content))
	for _, itemNode := range value.Content {
		var probe struct {
			Leaf    `yaml:",inline"`
			Service string       `yaml:"service,omitempty"`
			App     string       `yaml:"app,omitempty"`
			Run     string       `yaml:"run,omitempty"`
			Health  *HealthCheck `yaml:"health,omitempty"`
		}
		if err := decodeNodeStrict(itemNode, &probe); err != nil {
			return err
		}
		kind, title := "", probe.Title
		switch {
		case probe.Service != "" && probe.App != "":
			return fmt.Errorf("child: set either `service:` or `app:` — not both")
		case probe.Service != "":
			kind, title = "service", probe.Service
		case probe.App != "":
			kind, title = "app", probe.App
		}
		if title == "" {
			return fmt.Errorf("every child needs a name: start it with `- service: <name>`, `- app: <name>` or `- title: <name>`")
		}
		// the resolved name becomes the leaf's title for both syntaxes
		probe.Leaf.Title = title

		switch {
		// `- service:` — must be a link
		case kind == "service":
			if probe.Run != "" {
				return fmt.Errorf("service %q: a service is a link; executables belong in `app:` children", title)
			}
			if probe.URL == "" {
				return fmt.Errorf("service %q: needs a `url:`", title)
			}
			items = append(items, &Service{Leaf: probe.Leaf, Health: probe.Health})

		// `- app:` — must be a local executable
		case kind == "app":
			if probe.URL != "" {
				return fmt.Errorf("app %q: an app is launched locally — it needs `run:`, not `url:`", title)
			}
			if probe.Run == "" {
				return fmt.Errorf("app %q: needs a `run:` path", title)
			}
			if probe.Health != nil {
				return fmt.Errorf("app %q: healthchecks only apply to `url:` (service) children", title)
			}
			items = append(items, &App{Leaf: probe.Leaf, Run: probe.Run})

		// plain `- title:` — discriminate by url/run as before
		default:
			switch {
			case probe.URL != "" && probe.Run != "":
				return fmt.Errorf("child %q: set either `url:` (service) or `run:` (app) — not both", title)
			case probe.Run != "" && probe.Health != nil:
				return fmt.Errorf("child %q: healthchecks only apply to `url:` (service) children", title)
			case probe.URL != "":
				items = append(items, &Service{Leaf: probe.Leaf, Health: probe.Health})
			case probe.Run != "":
				items = append(items, &App{Leaf: probe.Leaf, Run: probe.Run})
			default:
				return fmt.Errorf("child %q: services need `url:`, apps need `run:` (one of the two)", title)
			}
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
