package mcpserver

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/piligrim/marvindocs/internal/chunk"
)

// get_docs is the Context7-style entry point: one call with a topic and
// a token budget returns the most relevant Markdown fragments across
// the matching pages, instead of a list of hits to fetch one by one.

const (
	getDocsDefaultTokens = 5000
	getDocsMaxTokens     = 50000
	// getDocsPages is how many top page hits are split into sections.
	getDocsPages = 8
)

type getDocsInput struct {
	Topic   string `json:"topic" jsonschema:"what to look up, e.g. 'jsonb operators' or 'useEffect cleanup'"`
	DocSlug string `json:"doc_slug,omitempty" jsonschema:"optional: restrict to this doc slug (see list_docs); otherwise detected from the topic"`
	Tokens  int    `json:"tokens,omitempty" jsonschema:"approximate token budget for the response (default 5000, max 50000)"`
}

// Fragment is one section of a page selected for the response.
type Fragment struct {
	DocSlug string `json:"doc_slug"`
	Path    string `json:"path"`
	Heading string `json:"heading,omitempty"`
	Content string `json:"content"`
	Tokens  int    `json:"tokens"`
}

type getDocsOutput struct {
	Topic     string     `json:"topic"`
	Tokens    int        `json:"tokens"`
	Fragments []Fragment `json:"fragments"`
}

func registerGetDocs(s *mcp.Server, d Deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "get_docs",
		Description: "Get documentation fragments about a topic within a token budget. " +
			"Searches page bodies, splits the best pages into sections by heading, " +
			"and returns the sections that mention the topic, most relevant first, " +
			"as Markdown with a 'Source:' line each. Prefer this over " +
			"search_pages + get_page when you just need an answer.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in getDocsInput) (*mcp.CallToolResult, getDocsOutput, error) {
		if strings.TrimSpace(in.Topic) == "" {
			return nil, getDocsOutput{}, fmt.Errorf("topic is required")
		}
		budget := in.Tokens
		if budget <= 0 {
			budget = getDocsDefaultTokens
		}
		if budget > getDocsMaxTokens {
			budget = getDocsMaxTokens
		}
		frags, err := d.collectFragments(ctx, in.Topic, in.DocSlug, budget)
		if err != nil {
			return nil, getDocsOutput{}, err
		}
		out := getDocsOutput{Topic: in.Topic, Fragments: frags}
		var text strings.Builder
		for k, f := range frags {
			out.Tokens += f.Tokens
			if k > 0 {
				text.WriteString("\n\n---\n\n")
			}
			fmt.Fprintf(&text, "Source: %s/%s\n\n%s", f.DocSlug, f.Path, f.Content)
		}
		if len(frags) == 0 {
			fmt.Fprintf(&text, "no documentation found for %q", in.Topic)
		}
		return textResult(text.String()), out, nil
	})
}

// collectFragments runs the page search, splits the top pages into
// sections, keeps those mentioning the topic and fills the token
// budget best-first. A section that does not fit is truncated and
// ends the selection.
func (d Deps) collectFragments(ctx context.Context, topic, docSlug string, budget int) ([]Fragment, error) {
	hits, err := d.Index.SearchPages(ctx, nil, topic, docSlug, getDocsPages)
	if err != nil {
		return nil, err
	}
	terms := chunk.Terms(topic)

	type scored struct {
		Fragment
		score int
		rank  int // page rank from the search, lower is better
		pos   int // section order within the page
	}
	var cands []scored
	for rank, h := range hits {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		html, err := d.Store.Page(h.DocSlug, h.Path)
		if err != nil {
			continue // stale index or missing db.json
		}
		md, err := d.markdown(h.DocSlug, h.Path, html)
		if err != nil {
			return nil, err
		}
		for pos, sec := range chunk.Split(md) {
			sc := chunk.Score(sec, terms)
			if sc == 0 {
				continue
			}
			cands = append(cands, scored{
				Fragment: Fragment{DocSlug: h.DocSlug, Path: h.Path, Heading: sec.Heading, Content: sec.Text},
				score:    sc, rank: rank, pos: pos,
			})
		}
	}
	sort.SliceStable(cands, func(a, b int) bool {
		if cands[a].score != cands[b].score {
			return cands[a].score > cands[b].score
		}
		if cands[a].rank != cands[b].rank {
			return cands[a].rank < cands[b].rank
		}
		return cands[a].pos < cands[b].pos
	})

	frags := []Fragment{}
	left := budget
	for _, c := range cands {
		if left <= 0 {
			break
		}
		f := c.Fragment
		f.Content = chunk.Truncate(f.Content, left)
		f.Tokens = chunk.Tokens(f.Content)
		if f.Tokens == 0 {
			break
		}
		frags = append(frags, f)
		left -= f.Tokens
	}
	return frags, nil
}
