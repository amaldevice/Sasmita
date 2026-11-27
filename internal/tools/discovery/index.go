// Package discovery implements ADR-0003: a frozen native tool set plus
// meta-tools that search and call other enabled tools. Nothing here changes the
// provider tool list while a conversation is in progress.
package discovery

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/amaldevice/sasmita/internal/tools"
)

// hit is one local full-text result. The full schema is returned so the model
// can call the tool without a second schema round trip (ADR-0003).
type hit struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
	Risk        string          `json:"risk"`
	Score       int             `json:"score"`
}

// Index is a small deterministic inverted index over enabled tool specs. It is
// local: no provider call, no SQL and no network are involved in a search.
type Index struct {
	entries []entry
}

type entry struct {
	spec   tools.Spec
	tokens map[string]int
}

// NewIndex tokenizes names, descriptions and schemas once. Weights favour the
// tool name, then its description, then schema field names.
func NewIndex(specs []tools.Spec) *Index {
	idx := &Index{entries: make([]entry, 0, len(specs))}
	for _, spec := range specs {
		tokens := make(map[string]int)
		for _, t := range tokenize(spec.Name) {
			tokens[t] += 6
		}
		for _, t := range tokenize(spec.Description) {
			tokens[t] += 3
		}
		for _, t := range tokenize(string(spec.InputSchema)) {
			tokens[t]++
		}
		idx.entries = append(idx.entries, entry{spec: spec, tokens: tokens})
	}
	return idx
}

// Search returns at most limit results, ranked by term coverage and score with
// name as a stable tie-break. Meta-tools are never returned to themselves.
func (idx *Index) Search(query string, limit int) []hit {
	if limit <= 0 {
		limit = 5
	}
	if limit > 5 {
		limit = 5
	}
	terms := tokenize(query)
	type ranked struct {
		hit   hit
		score int
	}
	results := make([]ranked, 0, len(idx.entries))
	for _, e := range idx.entries {
		if e.spec.Name == "find_tools" || e.spec.Name == "call_tool" {
			continue
		}
		score, matches := 0, 0
		for _, term := range terms {
			hits := e.tokens[term]
			if hits == 0 {
				// Prefix matches still help ("artifact" -> "artifacts").
				for token, weight := range e.tokens {
					if strings.HasPrefix(token, term) {
						hits += weight / 2
					}
				}
			}
			if hits > 0 {
				matches++
				score += hits
			}
		}
		if matches == 0 {
			continue
		}
		// Coverage dominates raw frequency so a query with several terms does
		// not collapse into whichever word appears most in one description.
		score += matches * 20
		results = append(results, ranked{
			hit: hit{
				Name: e.spec.Name, Description: e.spec.Description,
				InputSchema: append(json.RawMessage(nil), e.spec.InputSchema...),
				Risk: string(e.spec.Risk), Score: score,
			},
			score: score,
		})
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].score == results[j].score {
			return results[i].hit.Name < results[j].hit.Name
		}
		return results[i].score > results[j].score
	})
	out := make([]hit, 0, limit)
	for i := 0; i < len(results) && i < limit; i++ {
		out = append(out, results[i].hit)
	}
	return out
}

func tokenize(s string) []string {
	lower := strings.ToLower(s)
	fields := strings.FieldsFunc(lower, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_')
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if len(f) > 1 || f == "0" || f == "1" {
			out = append(out, f)
		}
	}
	return out
}
