# mdsite parser

One parser for Markdown documentation sites (Hugo, mkdocs, Docusaurus)
kept in git repositories. Every site is a row in the `sites` table in
`sites.go`: repositories and directories to fetch, content roots, skipped
sections, include directories and optional hooks.

Supported sites (`--list`): `docker`, `kubernetes_docs`, `helm`,
`prometheus`, `traefik`, `argo_cd`, `istio`, `opentelemetry`, `grafana`,
`loki`, `podman`, `vagrant`, `terraform_aws`, `terraform_azurerm`,
`terraform_google`, `terraform_kubernetes`, `terraform_helm`, `terragrunt`,
`terratest`, `tflint`, `terraform_docs`, `atlantis`, `pulumi`, `vault`,
`consul`, `nomad`, `packer` (the last four from `hashicorp/web-unified-docs`,
newest version directory).

## Build and run

```bash
cd parsers/mdsite && go build -o ../../bin/mdsite-parser .
./bin/mdsite-parser --site helm --out data/public/docs/helm --force
# or via the registry:
./scripts/run-parser.sh helm
```

| Flag | Default | Notes |
|---|---|---|
| `--site` | (required) | Site name from `--list`. |
| `--source` | shallow sparse `git clone` | Local checkout instead of the clone (single-source sites only). |
| `--out` | (required) | Output directory. |
| `--force` | `false` | Overwrite existing files. |

`git` must be installed; only the listed directories are checked out.

## What it does

- strips front matter, Hugo shortcodes, MDX/JSX components and Go
  template blocks; inlines `include`, `include-markdown`, `--8<--` and `{!path!}`
  snippets; applies Hugo `module.mounts`;
- renders Markdown with gomarkdown, one page per file;
- rewrites links between pages to relative ones; other site links become
  absolute; unresolved `.md` links point to the site home;
- adds an entry per page plus `<page>: <heading>` entries for H2/H3;
  entry types come from the top `SectionDepth` directories or a `Section`
  hook;
- per-site hooks: `Prepare` (edit the file set before parsing, e.g. pick
  the newest Vagrant version, evaluate Podman `<< if >>` blocks), `Clean`
  (site-specific Markdown fixes), `Extra` (additional pages, e.g. Docker
  CLI reference from YAML).

## Adding a site

Add a `Site` entry to `sites.go`, run the parser, check links and
leftover template syntax in the output, then register the slug in
`parsers/registry.json` and add it to a bundle in `bundles.yaml`.
