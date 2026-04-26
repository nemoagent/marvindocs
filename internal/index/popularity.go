package index

import "strings"

// popularDocSlugs lists doc slug prefixes that get a small boost in
// cross-doc search so mainstream stacks surface above exotic ones.
// The set roughly matches scripts/sync-docs.sh DEFAULT_SLUGS — the
// "default popular set" of docs we expect users to care about.
//
// A prefix matches a slug iff slug == prefix or slug starts with
// prefix+"~" (so "python" matches "python~3.13" but not "pythonic").
var popularDocSlugs = []string{
	"angular", "angularjs", "bash", "c", "cpp", "css",
	"django", "docker", "go", "html", "http",
	"javascript", "jinja", "jquery", "kubernetes",
	"markdown", "nginx", "node", "npm",
	"postgresql", "python", "react", "redis",
	"ruby", "rust", "sqlite", "typescript", "vue",
}

// popularityBoost returns the score adjustment (subtracted from bm25,
// since lower is better) for a doc slug. Returns 0 for non-popular
// docs; a small positive value for popular ones. The boost is
// intentionally modest — it only matters as a tiebreaker between
// otherwise-comparable hits.
func popularityBoost(slug string) float64 {
	for _, p := range popularDocSlugs {
		if slug == p || strings.HasPrefix(slug, p+"~") {
			return 0.5
		}
	}
	return 0
}
