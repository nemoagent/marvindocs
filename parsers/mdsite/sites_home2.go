package main

import (
	"path"
	"regexp"
	"strconv"
	"strings"
)

// Smart home, part 2: Frigate, ESPHome and HACS docs.

func init() {
	for k, v := range home2Sites {
		sites[k] = v
	}
}

var home2Sites = map[string]Site{
	"frigate": {
		Name: "Frigate", Slug: "frigate", Type: "frigate",
		Home: "https://docs.frigate.video/", Code: gh("blakeblackshear/frigate"),
		Attribution: `&copy; Frigate, Inc. Documentation from <a href="https://github.com/blakeblackshear/frigate">blakeblackshear/frigate</a>, licensed under the MIT License.`,
		Sources:     []Source{{Repo: gh("blakeblackshear/frigate"), Dirs: []string{"docs/docs"}}},
		Roots:       []Root{{"docs/docs", ""}},
		BaseURL:     "https://docs.frigate.video", URLPrefix: "/",
		Skip:    []string{"mdx"}, // Docusaurus template page
		Prepare: frigatePrepare,
		Section: func(p string) string { return sectionByDepth(p, 1) },
		Clean:   frigateClean,
	},
	"esphome": {
		Name: "ESPHome", Slug: "esphome", Type: "esphome",
		Home: "https://esphome.io/", Code: gh("esphome/esphome-docs"),
		Attribution: `&copy; ESPHome Authors. Documentation from <a href="https://github.com/esphome/esphome-docs">esphome/esphome-docs</a>, licensed under CC BY-NC-SA 4.0.`,
		Sources:     []Source{{Repo: gh("esphome/esphome-docs"), Dirs: []string{"src/content/docs"}}},
		Roots:       []Root{{"src/content/docs", ""}},
		BaseURL:     "https://esphome.io", URLPrefix: "/",
		Skip:    []string{"404", "blog", "changelog", "projects"},
		Section: func(p string) string { return sectionByDepth(p, 2) },
		Clean:   esphomeClean,
	},
	"hacs": {
		Name: "HACS", Slug: "hacs", Type: "hacs",
		Home: "https://hacs.xyz/", Code: gh("hacs/documentation"),
		Attribution: `&copy; HACS. Documentation from <a href="https://github.com/hacs/documentation">hacs/documentation</a>, licensed under the MIT License.`,
		Sources:     []Source{{Repo: gh("hacs/documentation"), Dirs: []string{"source"}}},
		Roots:       []Root{{"source", ""}},
		BaseURL:     "https://hacs.xyz", URLPrefix: "/",
		Skip: []string{"includes", "assets", "overrides"},
		Section: func(p string) string {
			if parts := strings.Split(p, "/"); parts[0] == "docs" && len(parts) > 2 {
				return humanize(parts[1])
			}
			return "General"
		},
		Clean: hacsClean,
	},
}

var (
	docusaurusIDRe   = regexp.MustCompile(`(?m)^(id|slug):\s*(\S+)\s*$`)
	frigateNavPathRe = regexp.MustCompile(`<NavPath\s+path="([^"]*)"\s*/>`)
	frigateFaqRe     = regexp.MustCompile(`<FaqItem\s[^>]*?question=(?:"([^"]*)"|\{<>(.*?)</>\})[^>]*>`)
	frigateTabsRe    = regexp.MustCompile(`(?s)<Tabs\b[^>]*>`)
	detailsRe        = regexp.MustCompile(`(?m)^[ \t]*</?details[^>]*>[ \t]*\n?`)
	summaryRe        = regexp.MustCompile(`(?s)[ \t]*<summary>(.*?)</summary>[ \t]*`)
	frigateTabRe     = regexp.MustCompile(`<TabItem\s+value="([^"]*)"(?:\s+label="([^"]*)")?[^>]*>`)
)

// frigatePrepare moves docs to the URL Docusaurus gives them: the
// front matter id replaces the file name and slug "/" is the site root.
func frigatePrepare(files map[string]string) {
	for f, body := range files {
		if path.Ext(f) != ".md" || !strings.HasPrefix(body, "---\n") {
			continue
		}
		fm, _, _ := strings.Cut(body[4:], "\n---\n")
		var id, slug string
		for _, m := range docusaurusIDRe.FindAllStringSubmatch(fm, -1) {
			if m[1] == "id" {
				id = strings.Trim(m[2], `"'`)
			} else {
				slug = m[2]
			}
		}
		dir := path.Dir(f)
		switch {
		case slug == "/":
			delete(files, f)
			files["docs/docs/index.md"] = body
		case id != "" && id != strings.TrimSuffix(path.Base(f), ".md"):
			delete(files, f)
			files[path.Join(dir, id)+".md"] = body
		}
	}
}

// flattenDetails turns <details> blocks into plain Markdown: the HTML
// block would otherwise hide the Markdown inside it from the renderer.
func flattenDetails(md string) string {
	md = detailsRe.ReplaceAllString(md, "")
	return summaryRe.ReplaceAllString(md, "\n**$1**\n")
}

// frigateClean renders the Docusaurus components of the Frigate docs:
// NavPath as bold text, FaqItem as a heading, TabItem as a label.
func frigateClean(md string, _ map[string]string) string {
	md = flattenDetails(md)
	md = frigateTabsRe.ReplaceAllString(md, "")
	md = frigateNavPathRe.ReplaceAllString(md, "**$1**")
	md = frigateFaqRe.ReplaceAllString(md, "\n### $1$2\n")
	return frigateTabRe.ReplaceAllStringFunc(md, func(s string) string {
		m := frigateTabRe.FindStringSubmatch(s)
		name := m[2]
		switch {
		case name != "":
		case m[1] == "ui" || m[1] == "yaml":
			name = strings.ToUpper(m[1])
		default:
			name = humanize(m[1])
		}
		return "\n**" + name + "**\n"
	})
}

var (
	apiLinkRe          = regexp.MustCompile(`<API(?:Ref|Class|Struct)\s+text="([^"]*)"[^>]*/>`)
	esphomeFigRe       = regexp.MustCompile(`(?s)<Figure\b[^>]*/>`)
	esphomeFigBody     = regexp.MustCompile(`(?s)<Figure\b[^>]*[^/]>.*?</Figure>`)
	esphomeThemed      = regexp.MustCompile(`(?s)<ThemedImage\b.*?/>`)
	esphomeImgRe       = regexp.MustCompile(`(?s)<Image\b.*?/>`)
	esphomeCapRe       = regexp.MustCompile(`caption="([^"]*)"`)
	esphomeTableRe     = regexp.MustCompile(`(?s)<ImgTable\s+items=\{\[(.*?)\]\}\s*/>`)
	esphomeRowRe       = regexp.MustCompile(`\["([^"]*)",\s*"([^"]*)"`)
	esphomeEnumRe      = regexp.MustCompile(`(?s)<EnumValues\s+values=\{\[(.*?)\]\}\s*/>`)
	esphomeValRe       = regexp.MustCompile(`value:\s*"([^"]*)"`)
	esphomeDefRe       = regexp.MustCompile(`default:\s*true`)
	esphomeItemFenceRe = regexp.MustCompile("(?m)^([-*] )(```.*)$")
	esphomeDescRe      = regexp.MustCompile(`description:\s*"((?:[^"\\]|\\.)*)"`)
)

// esphomeClean renders the Astro components of the ESPHome docs: API
// references as code, figures as their caption, ImgTable as a link
// list and EnumValues as a list of values. A list item that starts with
// a code fence gets a lead-in line, the Markdown renderer needs one.
func esphomeClean(md string, _ map[string]string) string {
	md = esphomeItemFenceRe.ReplaceAllString(md, "${1}Example:\n  ${2}")
	md = flattenDetails(md)
	md = apiLinkRe.ReplaceAllString(md, "`$1`")
	md = esphomeFigBody.ReplaceAllString(md, "")
	md = esphomeThemed.ReplaceAllString(md, "")
	md = esphomeFigRe.ReplaceAllStringFunc(md, func(s string) string {
		if m := esphomeCapRe.FindStringSubmatch(s); m != nil {
			return "*" + m[1] + "*"
		}
		return ""
	})
	md = esphomeImgRe.ReplaceAllString(md, "")
	md = esphomeTableRe.ReplaceAllStringFunc(md, func(s string) string {
		var out []string
		for _, m := range esphomeRowRe.FindAllStringSubmatch(s, -1) {
			out = append(out, "- ["+m[1]+"]("+m[2]+")")
		}
		return strings.Join(out, "\n")
	})
	return esphomeEnumRe.ReplaceAllStringFunc(md, func(s string) string {
		var out []string
		for _, row := range strings.Split(s, "\n") {
			m := esphomeValRe.FindStringSubmatch(row)
			if m == nil {
				continue
			}
			line := "- `" + m[1] + "`"
			if esphomeDefRe.MatchString(row) {
				line += " (default)"
			}
			if d := esphomeDescRe.FindStringSubmatch(row); d != nil {
				line += ": " + d[1]
			}
			out = append(out, line)
		}
		return strings.Join(out, "\n")
	})
}

// hacsCoreUI holds the Home Assistant and HACS UI strings that the docs
// take from the frontend translations with the coreui() macro.
var hacsCoreUI = map[string]string{
	"panel.config":                                 "Settings",
	"ui.panel.config.dashboard.about.main":         "About",
	"ui.panel.config.dashboard.dashboards.main":    "Dashboards",
	"ui.panel.config.dashboard.devices.main":       "Devices & services",
	"ui.panel.config.integrations.add_integration": "Add integration",
	"ui.panel.config.lovelace.resources.caption":   "Resources",
	"ui.panel.config.updates.caption":              "Updates",
	"ui.components.data-table.search":              "Search",
}

var (
	hacsMyRe      = regexp.MustCompile(`<!-- hacs:my\s+(\w+)(?:\s+title="(.*)")? -->`)
	hacsUIRe      = regexp.MustCompile(`\{\{\s*(?:coreui|hacsui)\(\s*["']([^"']+)["']\s*\)\s*\}\}`)
	hacsPageRe    = regexp.MustCompile(`\{\{\s*(?:page|config)\.[\w.]+\s*\}\}`)
	hacsMdSlashRe = regexp.MustCompile(`(\]\([^)\s]*\.md)/`)
	hacsSnippetRe = regexp.MustCompile(`--8<--\s+"([^":]+):(\d+)"`)
	hacsIconRe    = regexp.MustCompile(`:(?:material|fontawesome|octicons|wave)[\w-]*:(?:\{[^}]*\})?`)
	hacsAttrRe    = regexp.MustCompile(`\{:?\s*(?:\.[\w-]+|[\w-]+="[^"]*")[^}]*\}`)
)

// hacsClean expands the macros and shortcodes of the HACS docs.
func hacsClean(md string, files map[string]string) string {
	md = hacsSnippetRe.ReplaceAllStringFunc(md, func(s string) string {
		m := hacsSnippetRe.FindStringSubmatch(s)
		from, _ := strconv.Atoi(m[2])
		lines := strings.Split(files[m[1]], "\n")
		if from < 1 || from > len(lines) {
			return ""
		}
		return strings.Join(lines[from-1:], "\n")
	})
	md = hacsUIRe.ReplaceAllStringFunc(md, func(s string) string {
		key := hacsUIRe.FindStringSubmatch(s)[1]
		if v, ok := hacsCoreUI[key]; ok {
			return v
		}
		return humanize(key[strings.LastIndex(key, ".")+1:])
	})
	md = hacsMyRe.ReplaceAllStringFunc(md, func(s string) string {
		m := hacsMyRe.FindStringSubmatch(s)
		if m[2] != "" {
			return m[2]
		}
		return strings.ReplaceAll(m[1], "_", " ")
	})
	md = hacsMdSlashRe.ReplaceAllString(md, "$1")
	md = hacsPageRe.ReplaceAllString(md, "next")
	md = hacsIconRe.ReplaceAllString(md, "")
	return hacsAttrRe.ReplaceAllString(md, "")
}
