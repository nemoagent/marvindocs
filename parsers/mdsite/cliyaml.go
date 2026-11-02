package main

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// cliCommand is one docs/reference YAML file of docker-cli-docs-tool
// (docker CLI, buildx, compose).
type cliCommand struct {
	Command    string      `yaml:"command"`
	Aliases    string      `yaml:"aliases"`
	Short      string      `yaml:"short"`
	Long       string      `yaml:"long"`
	Usage      string      `yaml:"usage"`
	Options    []cliOption `yaml:"options"`
	Inherited  []cliOption `yaml:"inherited_options"`
	Examples   string      `yaml:"examples"`
	Deprecated bool        `yaml:"deprecated"`
	Hidden     bool        `yaml:"hidden"`
	Cname      []string    `yaml:"cname"`
}

type cliOption struct {
	Option      string `yaml:"option"`
	Shorthand   string `yaml:"shorthand"`
	ValueType   string `yaml:"value_type"`
	Default     string `yaml:"default_value"`
	Description string `yaml:"description"`
	Deprecated  bool   `yaml:"deprecated"`
	Hidden      bool   `yaml:"hidden"`
}

// cliPages renders every YAML command under dirs as a page
// "reference/cli/docker/<sub>/<command>", like docs.docker.com.
func cliPages(files map[string]string, dirs []string, section string, add func(Page)) error {
	keys := make([]string, 0)
	for f := range files {
		for _, d := range dirs {
			if strings.HasPrefix(f, d+"/") && path.Ext(f) == ".yaml" {
				keys = append(keys, f)
			}
		}
	}
	sort.Strings(keys)
	for _, f := range keys {
		var c cliCommand
		if err := yaml.Unmarshal([]byte(files[f]), &c); err != nil {
			return fmt.Errorf("%s: %w", f, err)
		}
		if c.Command == "" || c.Hidden {
			continue
		}
		add(Page{
			Path:    cliPath(c.Command),
			Title:   c.Command,
			Section: section,
			HTML:    mdToHTML(cleanMarkdown(Site{}, cliMarkdown(c), files, 0)),
		})
	}
	return nil
}

func cliPath(command string) string {
	return "reference/cli/" + strings.Join(strings.Fields(command), "/")
}

func cliMarkdown(c cliCommand) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", c.Command)
	if c.Deprecated {
		b.WriteString("**Deprecated.**\n\n")
	}
	fmt.Fprintf(&b, "%s\n\n", strings.TrimSpace(c.Short))
	if c.Usage != "" {
		fmt.Fprintf(&b, "```console\n%s\n```\n\n", strings.TrimSpace(c.Usage))
	}
	if c.Aliases != "" {
		fmt.Fprintf(&b, "Aliases: `%s`\n\n", strings.Join(strings.Split(c.Aliases, ", "), "`, `"))
	}
	if len(c.Cname) > 0 {
		b.WriteString("## Subcommands\n\n")
		for _, n := range c.Cname {
			rel := strings.TrimPrefix(cliPath(n), cliPath(c.Command)+"/")
			fmt.Fprintf(&b, "- [%s](%s)\n", n, path.Base(cliPath(c.Command))+"/"+rel)
		}
		b.WriteString("\n")
	}
	if long := strings.TrimSpace(c.Long); long != "" {
		fmt.Fprintf(&b, "## Description\n\n%s\n\n", long)
	}
	writeOptions(&b, "Options", c.Options)
	writeOptions(&b, "Global options", c.Inherited)
	if ex := strings.TrimSpace(c.Examples); ex != "" {
		fmt.Fprintf(&b, "## Examples\n\n%s\n", ex)
	}
	return b.String()
}

func writeOptions(b *strings.Builder, title string, opts []cliOption) {
	var rows []string
	for _, o := range opts {
		if o.Hidden {
			continue
		}
		name := "`--" + o.Option + "`"
		if o.Shorthand != "" {
			name = "`-" + o.Shorthand + "`, " + name
		}
		def := ""
		if o.Default != "" && o.Default != "false" && o.Default != "[]" && o.Default != "map[]" && o.Default != "0" {
			def = "`" + o.Default + "`"
		}
		desc := strings.Join(strings.Fields(o.Description), " ")
		if o.Deprecated {
			desc = "Deprecated. " + desc
		}
		rows = append(rows, fmt.Sprintf("| %s | %s | %s | %s |", name, o.ValueType, def, strings.ReplaceAll(desc, "|", `\|`)))
	}
	if len(rows) == 0 {
		return
	}
	fmt.Fprintf(b, "## %s\n\n| Option | Type | Default | Description |\n|---|---|---|---|\n%s\n\n", title, strings.Join(rows, "\n"))
}
