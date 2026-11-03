package main

import (
	"fmt"
	"regexp"
	"strings"
)

// Terraform ecosystem: provider docs, HashiCorp stack, wrappers and Pulumi.

func init() {
	for k, v := range tfSites {
		sites[k] = v
	}
}

var orderRe = regexp.MustCompile(`/\d+[-_]`)

// stripOrder drops numeric ordering prefixes ("03-features/01-units")
// from path segments of files under dir.
func stripOrder(dir string) func(map[string]string) {
	return func(files map[string]string) {
		for f, body := range files {
			rest, ok := strings.CutPrefix(f, dir)
			if !ok {
				continue
			}
			if clean := orderRe.ReplaceAllString("/"+rest, "/"); clean != "/"+rest {
				delete(files, f)
				files[dir+clean[1:]] = body
			}
		}
	}
}

const (
	hcAttr  = `&copy; HashiCorp, an IBM Company. Documentation from <a href="https://github.com/hashicorp/web-unified-docs">hashicorp/web-unified-docs</a>, licensed under the Mozilla Public License 2.0.`
	gwAttr  = `&copy; Gruntwork, Inc. Documentation from <a href="https://github.com/gruntwork-io/%[1]s">gruntwork-io/%[1]s</a>, licensed under the %[2]s.`
	tfpAttr = `&copy; HashiCorp, an IBM Company. Documentation from <a href="https://github.com/hashicorp/terraform-provider-%[1]s">hashicorp/terraform-provider-%[1]s</a>, licensed under the Mozilla Public License 2.0.`
)

// hashicorpSite builds a web-unified-docs product doc (newest version).
func hashicorpSite(product, name string) Site {
	return Site{
		Name: name, Slug: product, Type: product,
		Home: "https://developer.hashicorp.com/" + product + "/docs", Code: gh("hashicorp/web-unified-docs"),
		Attribution: hcAttr,
		Sources:     []Source{{Repo: gh("hashicorp/web-unified-docs"), Dirs: []string{"content/" + product}}},
		Roots:       []Root{{product, ""}},
		Skip:        []string{"partials"},
		IncludeDirs: []string{product + "/partials"},
		BaseURL:     "https://developer.hashicorp.com", URLPrefix: "/" + product + "/",
		Prepare:      latestVersion(product),
		SectionDepth: 2,
	}
}

// providerSite builds a Terraform provider doc from its repository docs dir.
func providerSite(name, docs string) Site {
	repo := "hashicorp/terraform-provider-" + name
	roots := []Root{}
	for _, r := range [][2]string{{"r", "resources"}, {"d", "data-sources"}} {
		roots = append(roots, Root{docs + "/" + r[0], r[1]})
	}
	roots = append(roots, Root{docs, ""})
	return Site{
		Name: "Terraform " + strings.ToUpper(name[:1]) + name[1:] + " Provider", Slug: "terraform_" + name, Type: "terraform_provider",
		Home: "https://registry.terraform.io/providers/hashicorp/" + name + "/latest/docs", Code: gh(repo),
		Attribution: fmt.Sprintf(tfpAttr, name),
		Sources:     []Source{{Repo: gh(repo), Dirs: []string{docs}}},
		Roots:       roots,
		Skip:        []string{"list-resources", "cdktf"},
		BaseURL:     "https://registry.terraform.io", URLPrefix: "/providers/hashicorp/" + name + "/latest/docs/",
	}
}

func gruntworkAttr(repo, license string) string { return fmt.Sprintf(gwAttr, repo, license) }

var tfSites = map[string]Site{
	"vault":  hashicorpSite("vault", "Vault"),
	"consul": hashicorpSite("consul", "Consul"),
	"nomad":  hashicorpSite("nomad", "Nomad"),
	"packer": hashicorpSite("packer", "Packer"),

	"terraform_aws":        providerSite("aws", "website/docs"),
	"terraform_azurerm":    providerSite("azurerm", "website/docs"),
	"terraform_google":     providerSite("google", "website/docs"),
	"terraform_kubernetes": providerSite("kubernetes", "docs"),
	"terraform_helm":       providerSite("helm", "docs"),

	"terragrunt": {
		Name: "Terragrunt", Slug: "terragrunt", Type: "terragrunt",
		Home: "https://terragrunt.gruntwork.io/docs/", Code: gh("gruntwork-io/terragrunt"),
		Attribution:  gruntworkAttr("terragrunt", "MIT License"),
		Sources:      []Source{{Repo: gh("gruntwork-io/terragrunt"), Dirs: []string{"docs/src/content/docs"}}},
		Roots:        []Root{{"docs/src/content/docs", ""}},
		Prepare:      stripOrder("docs/src/content/docs/"),
		SectionDepth: 2,
		BaseURL:      "https://terragrunt.gruntwork.io", URLPrefix: "/docs/",
	},
	"terratest": {
		Name: "Terratest", Slug: "terratest", Type: "terratest",
		Home: "https://terratest.gruntwork.io/docs/", Code: gh("gruntwork-io/terratest"),
		Attribution: gruntworkAttr("terratest", "Apache License 2.0"),
		Sources:     []Source{{Repo: gh("gruntwork-io/terratest"), Dirs: []string{"docs/_docs"}}},
		Roots:       []Root{{"docs/_docs", ""}},
		Prepare:     stripOrder("docs/_docs/"),
		Liquid:      true,
		BaseURL:     "https://terratest.gruntwork.io", URLPrefix: "/docs/",
	},
	"tflint": {
		Name: "TFLint", Slug: "tflint", Type: "tflint",
		Home: "https://github.com/terraform-linters/tflint/tree/master/docs", Code: gh("terraform-linters/tflint"),
		Attribution: `&copy; The TFLint Authors. Documentation from <a href="https://github.com/terraform-linters/tflint">terraform-linters/tflint</a>, licensed under the Mozilla Public License 2.0.`,
		Sources:     []Source{{Repo: gh("terraform-linters/tflint"), Dirs: []string{"docs"}}},
		Roots:       []Root{{"docs", ""}},
		BaseURL:     "https://github.com/terraform-linters/tflint/blob/master", URLPrefix: "/docs/",
	},
	"terraform_docs": {
		Name: "terraform-docs", Slug: "terraform_docs", Type: "terraform_docs",
		Home: "https://terraform-docs.io/", Code: gh("terraform-docs/terraform-docs"),
		Attribution: `&copy; The terraform-docs Authors. Documentation from <a href="https://github.com/terraform-docs/terraform-docs">terraform-docs/terraform-docs</a>, licensed under the MIT License.`,
		Sources:     []Source{{Repo: gh("terraform-docs/terraform-docs"), Dirs: []string{"docs"}}},
		Roots:       []Root{{"docs", ""}},
		Skip:        []string{"developer-guide"},
		BaseURL:     "https://terraform-docs.io", URLPrefix: "/",
	},
	"atlantis": {
		Name: "Atlantis", Slug: "atlantis", Type: "atlantis",
		Home: "https://www.runatlantis.io/docs/", Code: gh("runatlantis/atlantis"),
		Attribution: `&copy; The Atlantis Authors. Documentation from <a href="https://github.com/runatlantis/atlantis">runatlantis/atlantis</a>, licensed under the Apache License 2.0.`,
		Sources:     []Source{{Repo: gh("runatlantis/atlantis"), Dirs: []string{"runatlantis.io/docs"}}},
		Roots:       []Root{{"runatlantis.io/docs", ""}},
		BaseURL:     "https://www.runatlantis.io", URLPrefix: "/docs/",
	},
	"pulumi": {
		Name: "Pulumi", Slug: "pulumi", Type: "pulumi",
		Home: "https://www.pulumi.com/docs/", Code: gh("pulumi/docs"),
		Attribution:  `&copy; Pulumi Corporation. Documentation from <a href="https://github.com/pulumi/docs">pulumi/docs</a>, licensed under the Apache License 2.0.`,
		Sources:      []Source{{Repo: gh("pulumi/docs"), Dirs: []string{"content/docs"}}},
		Roots:        []Root{{"content/docs", ""}},
		Skip:         []string{"ai", "insights", "administration", "support", "idp", "reference"},
		SectionDepth: 2,
		BaseURL:      "https://www.pulumi.com", URLPrefix: "/docs/",
	},
}
