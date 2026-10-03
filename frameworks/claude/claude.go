package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"arxiv-weekly/config"
	"arxiv-weekly/domain"
)

const model = "claude-opus-5-5"
const maxTokens = 64000

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
		anthropic.BetaOutputConfigEffortMedium, shortlistSchema, &result)
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
		anthropic.BetaOutputConfigEffortHigh, pickSchema, &result)
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

// ask sends one request and decodes the JSON answer into out. It streams
// because the input is long, and opts into the server-side fallback so a week
// with a paper the safety classifiers decline doesn't end without an answer.
func ask(system, user string, effort anthropic.BetaOutputConfigEffort, schema map[string]any, out any) error {
	client := anthropic.NewClient(option.WithAPIKey(config.MustGet("ARXIV_WEEKLY_ANTHROPIC_API_KEY")))

	stream := client.Beta.Messages.NewStreaming(context.Background(), anthropic.BetaMessageNewParams{
		Model:     model,
		MaxTokens: maxTokens,
		Betas:     []anthropic.AnthropicBeta{anthropic.AnthropicBetaServerSideFallback2026_07_01},
		Fallbacks: anthropic.BetaFallbacksParamOfDefault(),
		System:    []anthropic.BetaTextBlockParam{{Text: system}},
		Messages: []anthropic.BetaMessageParam{
			anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(user)),
		},
		OutputConfig: anthropic.BetaOutputConfigParam{
			Effort: effort,
			Format: anthropic.BetaJSONOutputFormatParam{Schema: schema},
		},
	})

	message := anthropic.BetaMessage{}
	for stream.Next() {
		if err := message.Accumulate(stream.Current()); err != nil {
			return fmt.Errorf("claude: %w", err)
		}
	}
	if err := stream.Err(); err != nil {
		return fmt.Errorf("claude: %w", err)
	}

	log.Printf("claude: model %s, %d input tokens, %d output tokens",
		message.Model, message.Usage.InputTokens, message.Usage.OutputTokens)

	switch message.StopReason {
	case anthropic.BetaStopReasonRefusal:
		return fmt.Errorf("claude: request declined (%s)", message.StopDetails.Category)
	case anthropic.BetaStopReasonMaxTokens:
		return fmt.Errorf("claude: answer cut off at %d tokens", maxTokens)
	}

	var text strings.Builder
	for _, block := range message.Content {
		if variant, ok := block.AsAny().(anthropic.BetaTextBlock); ok {
			text.WriteString(variant.Text)
		}
	}
	if err := json.Unmarshal([]byte(text.String()), out); err != nil {
		return fmt.Errorf("claude: unreadable answer: %w", err)
	}
	return nil
}
