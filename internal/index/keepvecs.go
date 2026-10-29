package index

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// keepVersions is how many newest versions of a doc get vectors by
// default; older ones are searched lexically unless embedded explicitly.
const keepVersions = 3

// ltsVersions are old releases that keep vectors besides the newest
// keepVersions; node~*_lts slugs are recognised by their suffix.
var ltsVersions = map[string]bool{
	"openjdk~11": true, "openjdk~8": true, "openjdk~8_gui": true, "openjdk~8_web": true,
	"django~4.2": true, "django~3.2": true, "django~2.2": true, "django~1.11": true, "django~1.8": true,
	"qt~5.15": true, "qt~5.12": true, "qt~5.9": true, "qt~5.6": true,
	"laravel~6": true, "laravel~5.5": true, "laravel~5.1": true,
	"symfony~2.8": true, "symfony~2.7": true,
	"haproxy~3.0": true, "haproxy~2.8": true, "haproxy~2.6": true, "haproxy~2.4": true,
	"haproxy~2.2": true, "haproxy~2.0": true, "haproxy~1.8": true,
	"julia~1.6": true, "julia~1.0": true,
}

// versionGroup returns the key slugs are ranked within: the doc name plus
// the variant after "_" (gcc~14_cpp and gcc~13_cpp rank apart from
// gcc~14). ok is false when the part after "~" is not a version
// (haxe~cpp, rethinkdb~java): such slugs are variants and always kept.
func versionGroup(slug string) (key string, ok bool) {
	tilde := strings.Index(slug, "~")
	if tilde < 0 {
		return slug, true
	}
	name, ver := slug[:tilde], slug[tilde+1:]
	if ver == "" || ver[0] < '0' || ver[0] > '9' {
		return slug, false
	}
	if u := strings.Index(ver, "_"); u >= 0 && ver[u+1:] != "lts" {
		return name + "_" + ver[u+1:], true
	}
	return name, true
}

// vectorDocs picks the slugs that get vectors by default: the newest
// keepVersions of every version group, LTS releases and variants.
func vectorDocs(slugs []string) []string {
	groups := make(map[string][]string)
	var out []string
	for _, s := range slugs {
		key, ok := versionGroup(s)
		if !ok || strings.HasSuffix(s, "_lts") || ltsVersions[s] {
			out = append(out, s)
			continue
		}
		groups[key] = append(groups[key], s)
	}
	for _, g := range groups {
		sort.Slice(g, func(a, b int) bool { return compareVersions(g[a], g[b]) > 0 })
		if len(g) > keepVersions {
			g = g[:keepVersions]
		}
		out = append(out, g...)
	}
	sort.Strings(out)
	return out
}

// VectorDocs lists the indexed docs that get vectors by default (see
// vectorDocs); index embed without --doc embeds only these.
func (i *Index) VectorDocs(ctx context.Context) ([]string, error) {
	slugs, err := i.docSlugs(ctx)
	if err != nil {
		return nil, err
	}
	return vectorDocs(slugs), nil
}

// PruneVectors deletes the vectors of docs outside VectorDocs and returns
// those docs with the number of vectors each had. With dryRun nothing is
// deleted. The file only shrinks after Vacuum.
func (i *Index) PruneVectors(ctx context.Context, dryRun bool) (map[string]int, error) {
	keep, err := i.VectorDocs(ctx)
	if err != nil {
		return nil, err
	}
	kept := make(map[string]bool, len(keep))
	for _, s := range keep {
		kept[s] = true
	}
	rows, err := i.db.QueryContext(ctx,
		`SELECT p.doc_slug, COUNT(*) FROM chunk_vecs v
		 JOIN chunks c ON c.id = v.chunk_id JOIN pages p ON p.id = c.page_id
		 GROUP BY p.doc_slug`)
	if err != nil {
		return nil, err
	}
	pruned := make(map[string]int)
	for rows.Next() {
		var s string
		var n int
		if err := rows.Scan(&s, &n); err != nil {
			rows.Close()
			return nil, err
		}
		if !kept[s] {
			pruned[s] = n
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil || dryRun {
		return pruned, err
	}
	for s := range pruned {
		if _, err := i.db.ExecContext(ctx,
			`DELETE FROM chunk_vecs WHERE chunk_id IN (SELECT c.id FROM chunks c
			 JOIN pages p ON p.id = c.page_id WHERE p.doc_slug = ?)`, s); err != nil {
			return pruned, fmt.Errorf("prune vectors %s: %w", s, err)
		}
	}
	i.vecMu.Lock()
	i.vecCache = nil
	i.vecMu.Unlock()
	return pruned, nil
}

// Vacuum rewrites the database file to give freed pages back to the disk.
func (i *Index) Vacuum(ctx context.Context) error {
	_, err := i.db.ExecContext(ctx, `VACUUM`)
	return err
}
