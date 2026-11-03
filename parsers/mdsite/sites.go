package main

import (
	"maps"
	"path"
	"regexp"
	"strconv"
	"strings"
)

func gh(repo string) string { return "https://github.com/" + repo }

var codeSampleRe = regexp.MustCompile(`\{\{[<%]\s*code_sample\s+(?:language="(\w+)"\s+)?file="([^"]+)"[^}]*[%>]\}\}`)

// sites lists every supported documentation set by --site name.
var sites = map[string]Site{
	"docker": {
		Name: "Docker", Slug: "docker", Type: "docker",
		Home: "https://docs.docker.com/", Code: gh("docker/docs"),
		Attribution: `&copy; Docker Inc. Documentation from <a href="https://github.com/docker/docs">docker/docs</a>, licensed under the Apache License 2.0.`,
		Sources: []Source{{Repo: gh("docker/docs"), Dirs: []string{
			"content/manuals", "content/reference", "content/get-started", "content/guides",
			"content/includes", "data/cli/engine", "data/cli/buildx",
			"_vendor/github.com/docker/compose/v5/docs/reference",
			"_vendor/github.com/moby", "_vendor/github.com/docker/buildx", "_vendor/github.com/docker/cli",
		}}},
		Mounts: dockerMounts(),
		Roots: []Root{
			{"content/manuals", ""},
			{"content/reference", "reference"},
			{"content/get-started", "get-started"},
			{"content/guides", "guides"},
		},
		Skip: []string{
			"ai", "dhi", "accounts", "agentic-platform", "subscription-billing", "offload",
			"build-cloud", "support", "reference/samples",
		},
		IncludeDirs:  []string{"content/includes"},
		SectionDepth: 2,
		BaseURL:      "https://docs.docker.com", URLPrefix: "/",
		Extra: func(files map[string]string, add func(Page)) error {
			return cliPages(files, []string{
				"data/cli/engine", "data/cli/buildx",
				"_vendor/github.com/docker/compose/v5/docs/reference",
			}, "CLI reference", add)
		},
	},
	"kubernetes_docs": {
		Name: "Kubernetes Docs", Slug: "kubernetes_docs", Type: "kubernetes",
		Home: "https://kubernetes.io/docs/", Code: gh("kubernetes/website"),
		Attribution: `&copy; The Kubernetes Authors. Documentation from <a href="https://github.com/kubernetes/website">kubernetes/website</a>, licensed under CC BY 4.0.`,
		Sources: []Source{{Repo: gh("kubernetes/website"), Dirs: []string{
			"content/en/docs", "content/en/includes", "content/en/examples",
		}}},
		Roots:        []Root{{"content/en/docs", ""}},
		Skip:         []string{"contribute", "reference/kubernetes-api", "doc-contributor-tools", "home"},
		IncludeDirs:  []string{"content/en/includes"},
		SectionDepth: 2,
		BaseURL:      "https://kubernetes.io", URLPrefix: "/docs/",
		Clean: func(md string, files map[string]string) string {
			return codeSampleRe.ReplaceAllStringFunc(md, func(s string) string {
				m := codeSampleRe.FindStringSubmatch(s)
				body, ok := files["content/en/examples/"+m[2]]
				if !ok {
					return ""
				}
				lang := m[1]
				if lang == "" {
					lang = strings.TrimPrefix(path.Ext(m[2]), ".")
				}
				return "```" + lang + "\n" + strings.TrimRight(body, "\n") + "\n```"
			})
		},
	},
	"helm": {
		Name: "Helm", Slug: "helm", Type: "helm",
		Home: "https://helm.sh/docs/", Code: gh("helm/helm-www"),
		Attribution: `&copy; The Helm Authors. Documentation from <a href="https://github.com/helm/helm-www">helm/helm-www</a>, licensed under CC BY 4.0.`,
		Sources:     []Source{{Repo: gh("helm/helm-www"), Dirs: []string{"docs"}}},
		Roots:       []Root{{"docs", ""}},
		BaseURL:     "https://helm.sh", URLPrefix: "/docs/",
	},
}

// dockerMounts mirrors module.imports mounts of docker/docs hugo.yaml.
func dockerMounts() []Mount {
	v := "_vendor/github.com/"
	return []Mount{
		{v + "moby/moby/api/docs/CHANGELOG.md", "content/reference/api/engine/version-history.md"},
		{v + "moby/buildkit/frontend/dockerfile/docs/reference.md", "content/reference/dockerfile.md"},
		{v + "moby/buildkit/frontend/dockerfile/docs/rules", "content/reference/build-checks"},
		{v + "moby/buildkit/docs/buildkitd.toml.md", "content/manuals/build/buildkit/toml-configuration.md"},
		{v + "docker/buildx/docs/bake-reference.md", "content/manuals/build/bake/reference.md"},
		{v + "docker/buildx/docs/bake-stdlib.md", "content/manuals/build/bake/stdlib.md"},
		{v + "docker/cli/docs/extend", "content/manuals/engine/extend"},
		{v + "docker/cli/docs/deprecated.md", "content/manuals/engine/deprecated.md"},
		{v + "docker/cli/docs/reference/run.md", "content/manuals/engine/containers/run.md"},
		{v + "docker/cli/docs/reference/dockerd.md", "content/reference/cli/dockerd.md"},
	}
}

func init() {
	for k, v := range moreSites {
		sites[k] = v
	}
}

var (
	textOpenRe  = regexp.MustCompile(`\{\{<\s*text\s+(?:syntax=)?"?(\w+)"?[^>]*>\}\}`)
	textCloseRe = regexp.MustCompile(`\{\{<\s*/text\s*>\}\}`)
	boilerRe    = regexp.MustCompile(`\{\{<\s*boilerplate\s+"?([\w-]+)"?\s*>\}\}`)
	sharedRe    = regexp.MustCompile(`\{\{[<%]\s*docs/shared\s+(?:lookup|source)="([^"]+)"(?:\s+(?:lookup|source)="([^"]+)")?[^}]*[%>]\}\}`)
	podDirRe    = regexp.MustCompile(`(?m)^@@(option|include)\s+(\S+)\s*$`)
	podNoteRe   = regexp.MustCompile(`(?m)^####>.*\n`)
	podTokRe    = regexp.MustCompile(`(?s)<<(.*?)>>`)
	podAltRe    = regexp.MustCompile(`<<([^|<>]*)\|([^|<>]*)>>`)
	manHeadRe   = regexp.MustCompile(`^%\s*(\S+)\s+\d+\s*\n`)
	versionRe   = regexp.MustCompile(`<[A-Z]+_VERSION>`)
)

// istioClean turns {{< text lang >}} blocks into fences and inlines
// boilerplates.
func istioClean(md string, files map[string]string) string {
	md = textOpenRe.ReplaceAllString(md, "\n```$1")
	md = textCloseRe.ReplaceAllString(md, "```\n")
	return boilerRe.ReplaceAllStringFunc(md, func(s string) string {
		_, body := splitFrontMatter(files["content/en/boilerplates/"+boilerRe.FindStringSubmatch(s)[1]+".md"])
		return body
	})
}

// grafanaShared inlines {{< docs/shared lookup="x.md" source="loki" >}}
// fragments of the same repo.
func grafanaShared(self string) func(string, map[string]string) string {
	return func(md string, files map[string]string) string {
		md = versionRe.ReplaceAllString(md, "latest")
		return sharedRe.ReplaceAllStringFunc(md, func(s string) string {
			m := sharedRe.FindStringSubmatch(s)
			lookup, source := m[1], m[2]
			if strings.HasSuffix(source, ".md") {
				lookup, source = source, lookup
			}
			if source != "" && source != self {
				return ""
			}
			_, body := splitFrontMatter(files["docs/sources/shared/"+strings.TrimPrefix(lookup, "/")])
			return body
		})
	}
}

// podmanPrepare expands the .md.in man page templates the way
// hack/markdown-preprocess does and turns "% podman-run 1" into a title.
func podmanPrepare(files map[string]string) {
	const dir = "docs/source/markdown/"
	for f, body := range files {
		if !strings.HasPrefix(f, dir) || !strings.HasSuffix(f, ".md.in") {
			continue
		}
		name := strings.TrimSuffix(path.Base(f), ".1.md.in")
		sub := strings.ReplaceAll(strings.TrimPrefix(name, "podman-"), "-", " ")
		isPod := strings.HasPrefix(name, "podman-pod-")
		body = podDirRe.ReplaceAllStringFunc(body, func(s string) string {
			m := podDirRe.FindStringSubmatch(s)
			if m[1] == "include" {
				return files[path.Join(dir, m[2])]
			}
			opt, quadlet := strings.CutPrefix(m[2], "quadlet:")
			text := podTemplate(podNoteRe.ReplaceAllString(files[dir+"options/"+opt+".md"], ""), quadlet)
			text = strings.NewReplacer("<<fullsubcommand>>", "podman "+sub, "<<subcommand>>", sub).Replace(text)
			return podAltRe.ReplaceAllStringFunc(text, func(a string) string {
				m := podAltRe.FindStringSubmatch(a)
				if isPod {
					return m[2]
				}
				return m[1]
			})
		})
		files[strings.TrimSuffix(f, ".in")] = body
		delete(files, f)
	}
	for f, body := range files {
		if strings.HasPrefix(f, dir) && strings.HasSuffix(f, ".md") {
			if m := manHeadRe.FindStringSubmatch(body); m != nil {
				files[f] = "# " + strings.ReplaceAll(m[1], "-", " ") + "\n" + body[len(m[0]):]
			}
		}
	}
}

// podmanSection groups man pages by command group: podman-pod-create.1
// goes to "podman pod", podman-run.1 to "podman", *.5 to "File formats".
func podmanSection(p string) string {
	name := strings.TrimPrefix(p, "markdown/")
	switch {
	case strings.HasSuffix(name, ".5"):
		return "File formats"
	case !strings.HasPrefix(name, "podman"):
		return "General"
	}
	parts := strings.Split(strings.TrimSuffix(name, path.Ext(name)), "-")
	if len(parts) > 2 && podmanGroups[parts[1]] {
		return "podman " + parts[1]
	}
	return "podman"
}

var podmanGroups = map[string]bool{
	"artifact": true, "container": true, "farm": true, "generate": true, "healthcheck": true,
	"image": true, "kube": true, "machine": true, "manifest": true, "network": true, "pod": true,
	"quadlet": true, "secret": true, "system": true, "volume": true,
}

// podTemplate evaluates << if [not] is_quadlet >> / << else >> /
// << endif >> blocks and << "a" if is_quadlet else "b" >>; other
// tokens are kept.
func podTemplate(text string, quadlet bool) string {
	var out strings.Builder
	var stack []bool
	active := func() bool {
		for _, a := range stack {
			if !a {
				return false
			}
		}
		return true
	}
	truthy := func(cond string) bool {
		if c, ok := strings.CutPrefix(cond, "not "); ok {
			return strings.TrimSpace(c) != "is_quadlet" || !quadlet
		}
		return cond == "is_quadlet" && quadlet
	}
	pos := 0
	for _, m := range podTokRe.FindAllStringSubmatchIndex(text, -1) {
		if active() {
			out.WriteString(text[pos:m[0]])
		}
		pos = m[1]
		inner := strings.TrimSpace(text[m[2]:m[3]])
		switch {
		case strings.HasPrefix(inner, "if "):
			stack = append(stack, truthy(strings.TrimSpace(inner[3:])))
		case inner == "else" && len(stack) > 0:
			stack[len(stack)-1] = !stack[len(stack)-1]
		case inner == "endif" && len(stack) > 0:
			stack = stack[:len(stack)-1]
		case strings.Contains(inner, " if ") && strings.Contains(inner, " else "):
			then, rest, _ := strings.Cut(inner, " if ")
			cond, els, _ := strings.Cut(rest, " else ")
			if active() {
				pick := els
				if truthy(strings.TrimSpace(cond)) {
					pick = then
				}
				out.WriteString(strings.Trim(strings.TrimSpace(pick), `'"`))
			}
		default:
			if active() {
				out.WriteString(text[m[0]:m[1]])
			}
		}
	}
	out.WriteString(text[pos:])
	return out.String()
}

// latestVersion keeps only the newest content/<product>/vX.Y.Z/content
// tree of hashicorp/web-unified-docs, re-keyed as <product>/.
func latestVersion(product string) func(map[string]string) {
	return func(files map[string]string) {
		base := "content/" + product + "/"
		best := ""
		for f := range files {
			v, _, _ := strings.Cut(strings.TrimPrefix(f, base), "/")
			if strings.HasPrefix(f, base) && semverLess(best, v) {
				best = v
			}
		}
		kept := map[string]string{}
		for f, body := range files {
			if rest, ok := strings.CutPrefix(f, base+best+"/content/"); ok {
				kept[product+"/"+rest] = body
			}
			delete(files, f)
		}
		maps.Copy(files, kept)
	}
}

// semverLess compares "vX.Y.Z" strings numerically.
func semverLess(a, b string) bool {
	pa, pb := strings.Split(strings.TrimPrefix(a, "v"), "."), strings.Split(strings.TrimPrefix(b, "v"), ".")
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var x, y int
		if i < len(pa) {
			x, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			y, _ = strconv.Atoi(pb[i])
		}
		if x != y {
			return x < y
		}
	}
	return false
}

var moreSites = map[string]Site{
	"vagrant": {
		Name: "Vagrant", Slug: "vagrant", Type: "vagrant",
		Home: "https://developer.hashicorp.com/vagrant/docs", Code: gh("hashicorp/web-unified-docs"),
		Attribution: `&copy; HashiCorp, an IBM Company. Documentation from <a href="https://github.com/hashicorp/web-unified-docs">hashicorp/web-unified-docs</a>, licensed under the Mozilla Public License 2.0.`,
		Sources:     []Source{{Repo: gh("hashicorp/web-unified-docs"), Dirs: []string{"content/vagrant"}}},
		Roots:       []Root{{"vagrant/docs", "docs"}, {"vagrant/intro", "intro"}, {"vagrant/vagrant-cloud", "vagrant-cloud"}, {"vagrant/vmware", "vmware"}},
		BaseURL:     "https://developer.hashicorp.com", URLPrefix: "/vagrant/",
		Prepare:      latestVersion("vagrant"),
		SectionDepth: 2,
	},
	"prometheus": {
		Name: "Prometheus", Slug: "prometheus", Type: "prometheus",
		Home: "https://prometheus.io/docs/", Code: gh("prometheus/prometheus"),
		Attribution: `&copy; Prometheus Authors. Documentation from <a href="https://github.com/prometheus/docs">prometheus/docs</a> and <a href="https://github.com/prometheus/prometheus">prometheus/prometheus</a>, licensed under the Apache License 2.0.`,
		Sources: []Source{
			{Repo: gh("prometheus/docs"), Dirs: []string{"docs"}},
			{Repo: gh("prometheus/prometheus"), Dirs: []string{"docs"}, Prefix: "server/"},
		},
		Roots:   []Root{{"server/docs", "prometheus/latest"}, {"docs", ""}},
		BaseURL: "https://prometheus.io", URLPrefix: "/docs/",
	},
	"traefik": {
		Name: "Traefik", Slug: "traefik", Type: "traefik",
		Home: "https://doc.traefik.io/traefik/", Code: gh("traefik/traefik"),
		Attribution: `&copy; Traefik Labs. Documentation from <a href="https://github.com/traefik/traefik">traefik/traefik</a>, licensed under the MIT License.`,
		Sources:     []Source{{Repo: gh("traefik/traefik"), Dirs: []string{"docs/content"}}},
		Roots:       []Root{{"docs/content", ""}},
		Skip:        []string{"contributing"},
		IncludeDirs: []string{"docs/content", "docs"},
		BaseURL:     "https://doc.traefik.io", URLPrefix: "/traefik/",
	},
	"argo_cd": {
		Name: "Argo CD", Slug: "argo_cd", Type: "argo_cd",
		Home: "https://argo-cd.readthedocs.io/en/stable/", Code: gh("argoproj/argo-cd"),
		Attribution:  `&copy; The Argo Authors. Documentation from <a href="https://github.com/argoproj/argo-cd">argoproj/argo-cd</a>, licensed under the Apache License 2.0.`,
		Sources:      []Source{{Repo: gh("argoproj/argo-cd"), Ref: "release-3.5", Dirs: []string{"docs"}}},
		Roots:        []Root{{"docs", ""}},
		Skip:         []string{"proposals", "developer-guide", "snyk", "assets", "CONTRIBUTING", "SUPPORT", "bug_triage", "operator-manual/tested-kubernetes-versions"},
		SectionDepth: 2,
		IncludeDirs:  []string{"docs"},
		BaseURL:      "https://argo-cd.readthedocs.io", URLPrefix: "/en/stable/",
	},
	"istio": {
		Name: "Istio", Slug: "istio", Type: "istio",
		Home: "https://istio.io/latest/docs/", Code: gh("istio/istio.io"),
		Attribution: `&copy; Istio Authors. Documentation from <a href="https://github.com/istio/istio.io">istio/istio.io</a>, licensed under the Apache License 2.0.`,
		Sources:     []Source{{Repo: gh("istio/istio.io"), Dirs: []string{"content/en/docs", "content/en/boilerplates"}}},
		Roots:       []Root{{"content/en/docs", ""}},
		Skip:        []string{"releases"},
		BaseURL:     "https://istio.io/latest", URLPrefix: "/docs/",
		Clean: istioClean,
	},
	"opentelemetry": {
		Name: "OpenTelemetry", Slug: "opentelemetry", Type: "opentelemetry",
		Home: "https://opentelemetry.io/docs/", Code: gh("open-telemetry/opentelemetry.io"),
		Attribution:  `&copy; OpenTelemetry Authors. Documentation from <a href="https://github.com/open-telemetry/opentelemetry.io">open-telemetry/opentelemetry.io</a>, licensed under CC BY 4.0.`,
		Sources:      []Source{{Repo: gh("open-telemetry/opentelemetry.io"), Dirs: []string{"content/en/docs"}}},
		Roots:        []Root{{"content/en/docs", ""}},
		Skip:         []string{"contributing", "demo", "_includes"},
		IncludeDirs:  []string{"content/en/docs/_includes"},
		SectionDepth: 2,
		BaseURL:      "https://opentelemetry.io", URLPrefix: "/docs/",
	},
	"grafana": {
		Name: "Grafana", Slug: "grafana", Type: "grafana",
		Home: "https://grafana.com/docs/grafana/latest/", Code: gh("grafana/grafana"),
		Attribution:  `&copy; Grafana Labs. Documentation from <a href="https://github.com/grafana/grafana">grafana/grafana</a>, licensed under CC BY-SA 4.0.`,
		Sources:      []Source{{Repo: gh("grafana/grafana"), Dirs: []string{"docs/sources"}}},
		Roots:        []Root{{"docs/sources", ""}},
		Skip:         []string{"shared", "whatsnew", "release-notes", "upgrade-guide"},
		SectionDepth: 2,
		BaseURL:      "https://grafana.com", URLPrefix: "/docs/grafana/latest/",
		Clean: grafanaShared("grafana"),
	},
	"loki": {
		Name: "Loki", Slug: "loki", Type: "loki",
		Home: "https://grafana.com/docs/loki/latest/", Code: gh("grafana/loki"),
		Attribution: `&copy; Grafana Labs. Documentation from <a href="https://github.com/grafana/loki">grafana/loki</a>, licensed under CC BY-SA 4.0.`,
		Sources:     []Source{{Repo: gh("grafana/loki"), Dirs: []string{"docs/sources"}}},
		Roots:       []Root{{"docs/sources", ""}},
		Skip:        []string{"shared", "release-notes", "community"},
		BaseURL:     "https://grafana.com", URLPrefix: "/docs/loki/latest/",
		Clean: grafanaShared("loki"),
	},
	"podman": {
		Name: "Podman", Slug: "podman", Type: "podman",
		Home: "https://docs.podman.io/en/latest/", Code: gh("containers/podman"),
		Attribution: `&copy; The Podman Authors. Documentation from <a href="https://github.com/containers/podman">containers/podman</a>, licensed under the Apache License 2.0.`,
		Sources:     []Source{{Repo: gh("containers/podman"), Dirs: []string{"docs/source/markdown"}}},
		Roots:       []Root{{"docs/source/markdown", "markdown"}},
		Skip:        []string{"markdown/options", "markdown/links"},
		BaseURL:     "https://docs.podman.io", URLPrefix: "/en/latest/",
		Prepare: podmanPrepare,
		Section: podmanSection,
	},
}
