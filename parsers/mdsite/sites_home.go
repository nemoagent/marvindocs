package main

import (
	"encoding/json"
	"path"
	"regexp"
	"strings"
)

// Smart home: Home Assistant, Zigbee2MQTT and Mosquitto docs.

func init() {
	for k, v := range homeSites {
		sites[k] = v
	}
}

var homeSites = map[string]Site{
	"home_assistant": {
		Name: "Home Assistant", Slug: "home_assistant", Type: "home_assistant",
		Home: "https://www.home-assistant.io/docs/", Code: gh("home-assistant/home-assistant.io"),
		Attribution: `&copy; Home Assistant Authors. Documentation from <a href="https://github.com/home-assistant/home-assistant.io">home-assistant/home-assistant.io</a>, licensed under CC BY-NC-SA 4.0.`,
		Sources: []Source{{Repo: gh("home-assistant/home-assistant.io"), Dirs: []string{
			"source/_docs", "source/_integrations", "source/_dashboards", "source/_actions",
			"source/_triggers", "source/_conditions", "source/_template_functions",
			"source/_building_blocks", "source/_faq", "source/_includes", "source/_data/integrations",
			"source/getting-started", "source/installation", "source/common-tasks",
			"source/voice_control", "source/dashboards", "source/cloud", "source/faq",
			"source/android", "source/ios", "source/more-info",
		}}},
		Roots: []Root{
			{"source/_docs", "docs"},
			{"source/_integrations", "integrations"},
			{"source/_dashboards", "dashboards"},
			{"source/_actions", "actions"},
			{"source/_triggers", "triggers"},
			{"source/_conditions", "conditions"},
			{"source/_template_functions", "template-functions"},
			{"source/_building_blocks", "building-blocks"},
			{"source/_faq", "faq"},
			{"source/getting-started", "getting-started"},
			{"source/installation", "installation"},
			{"source/common-tasks", "common-tasks"},
			{"source/voice_control", "voice_control"},
			{"source/dashboards", "dashboards"},
			{"source/cloud", "cloud"},
			{"source/faq", "faq"},
			{"source/android", "android"},
			{"source/ios", "ios"},
			{"source/more-info", "more-info"},
		},
		BaseURL: "https://www.home-assistant.io", URLPrefix: "/",
		Skip:    []string{"integrations/_integration_docs_template", "docs/configuration/entities_domains"},
		Prepare: haIntegrationTitles,
		Section: haSection,
		Clean:   haClean,
	},
	"home_assistant_dev": {
		Name: "Home Assistant Developers", Slug: "home_assistant_dev", Type: "home_assistant",
		Home: "https://developers.home-assistant.io/docs/", Code: gh("home-assistant/developers.home-assistant"),
		Attribution:  `&copy; Home Assistant Authors. Documentation from <a href="https://github.com/home-assistant/developers.home-assistant">home-assistant/developers.home-assistant</a>, licensed under CC BY-NC-SA 4.0.`,
		Sources:      []Source{{Repo: gh("home-assistant/developers.home-assistant"), Dirs: []string{"docs"}}},
		Roots:        []Root{{"docs", ""}},
		SectionDepth: 1,
		BaseURL:      "https://developers.home-assistant.io", URLPrefix: "/docs/",
		Clean: func(md string, _ map[string]string) string { return apiEndpointRe.ReplaceAllString(md, "**$2 $1**\n") },
	},
	"zigbee2mqtt": {
		Name: "Zigbee2MQTT", Slug: "zigbee2mqtt", Type: "zigbee2mqtt",
		Home: "https://www.zigbee2mqtt.io/", Code: gh("Koenkk/zigbee2mqtt.io"),
		Attribution: `&copy; Koen Kanters and contributors. Documentation from <a href="https://github.com/Koenkk/zigbee2mqtt.io">Koenkk/zigbee2mqtt.io</a>, licensed under the GNU General Public License 3.0.`,
		Sources:     []Source{{Repo: gh("Koenkk/zigbee2mqtt.io"), Dirs: []string{"docs/guide", "docs/advanced", "docs/devices"}}},
		Roots:       []Root{{"docs", ""}},
		BaseURL:     "https://www.zigbee2mqtt.io", URLPrefix: "/",
		Prepare: func(files map[string]string) {
			for f, body := range files { // device aliases are redirect stubs
				if strings.HasPrefix(body, "---\nredirectTo:") {
					delete(files, f)
				}
			}
		},
		Section: func(p string) string {
			if first, _, _ := strings.Cut(p, "/"); first == "devices" {
				return "Devices"
			}
			return sectionByDepth(p, 2)
		},
	},
	"mosquitto": {
		Name: "Mosquitto", Slug: "mosquitto", Type: "mosquitto",
		Home: "https://mosquitto.org/documentation/", Code: gh("eclipse-mosquitto/mosquitto"),
		Attribution: `&copy; Eclipse Mosquitto Authors. Man pages from <a href="https://github.com/eclipse-mosquitto/mosquitto">eclipse-mosquitto/mosquitto</a>, licensed under the Eclipse Public License 2.0 or the Eclipse Distribution License 1.0.`,
		Sources:     []Source{{Repo: gh("eclipse-mosquitto/mosquitto"), Dirs: []string{"man"}}},
		Roots:       []Root{{"man", ""}},
		BaseURL:     "https://mosquitto.org", URLPrefix: "/man/",
		Prepare: mosquittoPrepare,
		Section: func(string) string { return "Man pages" },
	},
}

// sectionByDepth returns the humanized first parts of a page path.
func sectionByDepth(p string, depth int) string {
	parts := strings.Split(p, "/")
	if len(parts) <= 1 {
		return "General"
	}
	if len(parts)-1 < depth {
		depth = len(parts) - 1
	}
	for i := range parts[:depth] {
		parts[i] = humanize(parts[i])
	}
	return strings.Join(parts[:depth], ": ")
}

var apiEndpointRe = regexp.MustCompile(`<ApiEndpoint\s+path="([^"]*)"\s+method="([^"]*)"\s*>`)

func haSection(p string) string {
	first, _, _ := strings.Cut(p, "/")
	switch first {
	case "docs":
		return sectionByDepth(p, 2)
	case "faq":
		return "FAQ"
	}
	return humanize(first)
}

// haIntegrationTitles adds the title that the Jekyll plugin takes from
// _data/integrations/<domain>.json to integration pages without one, and
// passes the title to includes that default to the page title.
func haIntegrationTitles(files map[string]string) {
	for f, body := range files {
		domain, ok := strings.CutPrefix(f, "source/_integrations/")
		domain, isMD := strings.CutSuffix(domain, ".markdown")
		if !ok || !isMD {
			continue
		}
		title, rest := splitFrontMatter(body)
		if title == "" {
			var meta struct{ Name string }
			if json.Unmarshal([]byte(files["source/_data/integrations/"+domain+".json"]), &meta) != nil || meta.Name == "" {
				continue
			}
			title = meta.Name
			q, _ := json.Marshal(title)
			if rest == body {
				body = "---\n---\n" + body
			}
			body = "---\ntitle: " + string(q) + "\n" + strings.TrimPrefix(body, "---\n")
		}
		files[f] = haBareIncludeRe.ReplaceAllString(body, `{% include $1 name="`+strings.ReplaceAll(title, `"`, "")+`" %}`)
	}
}

var (
	haRawRe         = regexp.MustCompile(`(?s)\{%-?\s*raw\s*-?%\}(.*?)\{%-?\s*endraw\s*-?%\}`)
	haJinjaRe       = regexp.MustCompile(`(?s)\{%-?\s*jinja\s*-?%\}(.*?)\{%-?\s*endjinja\s*-?%\}`)
	haExampleRe     = regexp.MustCompile(`(?s)([ \t]*)\{%-?\s*example\s*-?%\}\n?(.*?)\{%-?\s*endexample\s*-?%\}`)
	haCommentRe     = regexp.MustCompile(`(?s)\{%-?\s*comment\s*-?%\}.*?\{%-?\s*endcomment\s*-?%\}`)
	haYAMLRe        = regexp.MustCompile(`(?s)([ \t]*)\{%-?\s*(?:configuration_basic|configuration|options_yaml|options_ui|function_parameters|template_function_usage)\b[^%]*%\}\n?(.*?)\{%-?\s*end(?:configuration_basic|configuration|options_yaml|options_ui|function_parameters|template_function_usage)\s*-?%\}`)
	haIncludeRe     = regexp.MustCompile(`\{%-?\s*include\s+([^\s%]+)((?:[^%]|%[^}])*)%\}`)
	haParamRe       = regexp.MustCompile(`(\w+)=(?:"([^"]*)"|'([^']*)'|(\S+))`)
	haVarRe         = regexp.MustCompile(`\{\{-?\s*(include|page|site)\.[^}]*\}\}`)
	haIncVarRe      = regexp.MustCompile(`\{\{-?\s*include\.(\w+)\s*(?:\|\s*default:[^}]*)?\}\}`)
	haFlowRe        = regexp.MustCompile(`\{%-?\s*(if|unless|elsif|else|endif|endunless|case|when|endcase)\b\s*([^%]*?)\s*-?%\}`)
	haCaptureRe     = regexp.MustCompile(`(?s)\{%-?\s*capture\s+\w+\s*-?%\}.*?\{%-?\s*endcapture\s*-?%\}`)
	haVarNameRe     = regexp.MustCompile(`\{\{-?\s*(name|domain)\s*(\|[^}]*)?\}\}`)
	haBareIncludeRe = regexp.MustCompile(`\{%-?\s*include\s+(integrations/(?:config_flow|building_block_integration|supported_brand)\.md)\s*-?%\}`)
	haDetailsRe     = regexp.MustCompile(`\{%-?\s*details(?:\s+"([^"]*)")?[^%]*%\}`)
	haNoteRe        = regexp.MustCompile(`\{%-?\s*(note|important|tip|warning|caution|labs)(?:\s+title="([^"]*)")?[^%]*%\}`)
	haTermRe        = regexp.MustCompile(`\{%-?\s*term\s+(?:"([^"]+)"|(\w+))(?:\s+(?:"([^"]+)"|(\w+)))?\s*-?%\}`)
	haMyRe          = regexp.MustCompile(`\{%-?\s*my\s+(\w+)(?:[^%]*?\btitle="([^"]*)")?[^%]*%\}`)
	haLinkRe        = regexp.MustCompile(`\{%-?\s*active_link\s+(\S+)\s+([^%]*?)\s*%\}`)
	haEndRe         = regexp.MustCompile(`\{%-?\s*end(?:details|note|important|tip|warning|caution|labs)\s*-?%\}`)
	haTagRe         = regexp.MustCompile(`\{%-?.*?-?%\}`)
)

// haClean turns Jekyll/Liquid markup of the Home Assistant docs into
// plain Markdown. Raw blocks, Jinja examples and code blocks of the
// plugins are protected while Liquid tags are removed.
func haClean(md string, files map[string]string) string {
	return haCleanDepth(md, files, 0)
}

func haCleanDepth(md string, files map[string]string, depth int) string {
	var keep []string
	protect := func(s string) string {
		keep = append(keep, s)
		return "\x00" + string(rune('A'+len(keep)-1)) + "\x00"
	}
	md = haRawRe.ReplaceAllStringFunc(md, func(s string) string { return protect(haRawRe.FindStringSubmatch(s)[1]) })
	md = haJinjaRe.ReplaceAllStringFunc(md, func(s string) string {
		return protect("`" + strings.TrimSpace(haJinjaRe.FindStringSubmatch(s)[1]) + "`")
	})
	md = haCommentRe.ReplaceAllString(md, "")
	md = haExampleRe.ReplaceAllStringFunc(md, func(s string) string {
		m := haExampleRe.FindStringSubmatch(s)
		return protect(m[1] + "```yaml\n" + strings.TrimRight(m[2], "\n") + "\n" + m[1] + "```")
	})
	md = haYAMLRe.ReplaceAllStringFunc(md, func(s string) string {
		m := haYAMLRe.FindStringSubmatch(s)
		return protect(m[1] + "```yaml\n" + strings.TrimRight(haTagRe.ReplaceAllString(m[2], ""), "\n") + "\n" + m[1] + "```")
	})
	md = haIncludeRe.ReplaceAllStringFunc(md, func(s string) string {
		m := haIncludeRe.FindStringSubmatch(s)
		body, ok := files["source/_includes/"+m[1]]
		if !ok || depth >= 2 || path.Ext(m[1]) != ".md" || strings.Contains(body, "{% for") {
			return ""
		}
		params := map[string]string{}
		for _, p := range haParamRe.FindAllStringSubmatch(m[2], -1) {
			params[p[1]] = p[2] + p[3] + p[4]
		}
		_, body = splitFrontMatter(body)
		body = haCaptureRe.ReplaceAllString(body, "")
		body = haVarNameRe.ReplaceAllStringFunc(body, func(v string) string {
			m := haVarNameRe.FindStringSubmatch(v)
			if m[1] == "name" && params["name"] == "" {
				return "this integration"
			}
			if strings.Contains(m[2], "downcase") {
				return strings.ToLower(params[m[1]])
			}
			return params[m[1]]
		})
		body = haIncVarRe.ReplaceAllStringFunc(body, func(v string) string {
			return params[haIncVarRe.FindStringSubmatch(v)[1]]
		})
		body = haResolveFlow(body, params)
		return haCleanDepth(body, files, depth+1)
	})
	md = haVarRe.ReplaceAllString(md, "")
	md = haDetailsRe.ReplaceAllStringFunc(md, func(s string) string {
		title := haDetailsRe.FindStringSubmatch(s)[1]
		if title == "" {
			title = "More information"
		}
		return "**" + title + "**\n"
	})
	md = haNoteRe.ReplaceAllStringFunc(md, func(s string) string {
		m := haNoteRe.FindStringSubmatch(s)
		return label(m[1], m[2]) + "\n"
	})
	md = haEndRe.ReplaceAllString(md, "")
	md = haTermRe.ReplaceAllStringFunc(md, func(s string) string {
		m := haTermRe.FindStringSubmatch(s)
		for _, v := range []string{m[3], m[4], m[1], m[2]} {
			if v != "" {
				return v
			}
		}
		return ""
	})
	md = haMyRe.ReplaceAllStringFunc(md, func(s string) string {
		m := haMyRe.FindStringSubmatch(s)
		if m[2] != "" {
			return m[2]
		}
		return strings.ReplaceAll(m[1], "_", " ")
	})
	md = haLinkRe.ReplaceAllString(md, "[$2]($1)")
	md = haTagRe.ReplaceAllString(md, "")
	for i := len(keep) - 1; i >= 0; i-- { // later blocks may contain earlier ones
		md = strings.Replace(md, "\x00"+string(rune('A'+i))+"\x00", keep[i], 1)
	}
	return md
}

var haIncCondRe = regexp.MustCompile(`^include\.(\w+)(?:\s*==\s*["']([^"']*)["'])?$`)

// haResolveFlow evaluates if/unless/case blocks of an included file.
// Conditions on include parameters are evaluated, any other condition
// (page and site data) counts as false.
func haResolveFlow(body string, params map[string]string) string {
	var out strings.Builder
	type frame struct{ live, taken, parent bool }
	var stack []frame
	live := func() bool { return len(stack) == 0 || stack[len(stack)-1].live }
	eval := func(cond string) bool {
		m := haIncCondRe.FindStringSubmatch(cond)
		switch {
		case m == nil:
			return false
		case m[2] != "":
			return params[m[1]] == m[2]
		}
		return params[m[1]] != ""
	}
	pos := 0
	for _, loc := range haFlowRe.FindAllStringSubmatchIndex(body, -1) {
		if live() {
			out.WriteString(body[pos:loc[0]])
		}
		pos = loc[1]
		tag, cond := body[loc[2]:loc[3]], body[loc[4]:loc[5]]
		switch tag {
		case "if", "unless", "case":
			parent := live()
			ok := parent && tag != "case" && eval(cond) == (tag == "if")
			stack = append(stack, frame{live: ok, taken: ok, parent: parent})
		case "elsif", "when", "else":
			if len(stack) == 0 {
				continue
			}
			f := &stack[len(stack)-1]
			ok := f.parent && !f.taken && (tag == "else" || eval(cond))
			f.live = ok
			f.taken = f.taken || ok
		default:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	if live() {
		out.WriteString(body[pos:])
	}
	return out.String()
}
