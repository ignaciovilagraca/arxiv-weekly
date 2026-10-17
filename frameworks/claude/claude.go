package claude

import (
	"fmt"
	"strings"

	"arxiv-weekly/domain"
)

const shortlistPrompt = `You are helping a reader choose what to read from this week's arXiv AI papers.

The reader's interest profile:

<interests>
%s
</interests>

The user message lists every paper submitted this week as "number [primary category] title". Select the %d papers most likely to be worth this reader's time.

You only see titles here. A later pass reads the abstracts of the papers you select and picks the final few, so favour recall: when a title is ambiguous but could be an important result, include it.`

const pickPrompt = `You are helping a reader choose what to read from this week's arXiv AI papers.

The reader's interest profile:

<interests>
%s
</interests>

The user message holds the candidate papers, already filtered from the whole week, each with its number, primary category, title and abstract. Pick the %d papers the reader should read this weekend, ordered from most to least recommended. Prefer some variety of topics unless one topic clearly dominates the week.

For each pick write "why" in neutral Spanish, in at most two sentences: what the paper shows and why it matters to this reader. Stick to what the abstract actually claims, since the reader will use it to decide whether to open the paper.`

var shortlistSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"numbers": map[string]any{
			"type":  "array",
			"items": map[string]any{"type": "integer"},
		},
	},
	"required":             []string{"numbers"},
	"additionalProperties": false,
}

var pickSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"papers": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"number": map[string]any{"type": "integer"},
					"why":    map[string]any{"type": "string"},
				},
				"required":             []string{"number", "why"},
				"additionalProperties": false,
			},
		},
	},
	"required":             []string{"papers"},
	"additionalProperties": false,
}

// Shortlist narrows the whole week down to at most n candidates, judging by
// title alone: the abstracts of a full week don't fit in one request.
func Shortlist(papers []domain.Paper, interests string, n int) ([]domain.Paper, error) {
	var list strings.Builder
	for i, paper := range papers {
		fmt.Fprintf(&list, "%d [%s] %s\n", i+1, paper.Category, paper.Title)
	}

	var result struct {
		Numbers []int `json:"numbers"`
	}
	err := ask(fmt.Sprintf(shortlistPrompt, interests, n), list.String(),
		"medio", shortlistSchema, &result)
	if err != nil {
		return nil, err
	}

	var shortlist []domain.Paper
	seen := map[int]bool{}
	for _, number := range result.Numbers {
		if number < 1 || number > len(papers) || seen[number] || len(shortlist) == n {
			continue
		}
		seen[number] = true
		shortlist = append(shortlist, papers[number-1])
	}
	return shortlist, nil
}

// Pick reads the abstracts of the candidates and returns the n to read, best
// first.
func Pick(papers []domain.Paper, interests string, n int) ([]domain.Recommendation, error) {
	var list strings.Builder
	for i, paper := range papers {
		fmt.Fprintf(&list, "<paper number=\"%d\" category=\"%s\">\n<title>%s</title>\n<abstract>%s</abstract>\n</paper>\n",
			i+1, paper.Category, paper.Title, paper.Abstract)
	}

	var result struct {
		Papers []struct {
			Number int    `json:"number"`
			Why    string `json:"why"`
		} `json:"papers"`
	}
	err := ask(fmt.Sprintf(pickPrompt, interests, n), list.String(),
		"alto", pickSchema, &result)
	if err != nil {
		return nil, err
	}

	var picks []domain.Recommendation
	seen := map[int]bool{}
	for _, pick := range result.Papers {
		if pick.Number < 1 || pick.Number > len(papers) || seen[pick.Number] || len(picks) == n {
			continue
		}
		seen[pick.Number] = true
		picks = append(picks, domain.Recommendation{Paper: papers[pick.Number-1], Why: pick.Why})
	}
	return picks, nil
}

// ask gets the JSON answer to one request and decodes it into out, through llm: which model answers, and the fallbacks
// (for example a CLI, then a batch API at half price, then the direct API) are llm's
// business. The weekly run tolerates hours of waiting, so it asks with --sin-apuro.
func ask(system, user, effort string, schema map[string]any, out any) error {
	return askLLM(system, user, effort, schema, out)
}
