package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"arxiv-weekly/config"
	"arxiv-weekly/domain"
)

const model = "claude-opus-5-5"
const maxTokens = 64000

// A batch expires on its own after 24 hours.
const batchDeadline = 25 * time.Hour
const pollInterval = time.Minute

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

// ask gets the JSON answer to one request and decodes it into out. It asks
// the API; when the API key has run out of credits it asks the claude CLI
// instead, which is billed to the Claude subscription.
func ask(system, user string, effort anthropic.BetaOutputConfigEffort, schema map[string]any, out any) error {
	err := askAPI(system, user, effort, schema, out)
	if err != nil && outOfCredits(err) {
		log.Printf("%v", err)
		log.Printf("claude: the API key is out of credits, asking through the claude CLI")
		return askCLI(system, user, string(effort), schema, out)
	}
	return err
}

// outOfCredits tells an API key without balance from any other failure. The
// API answers "Your credit balance is too low to access the Anthropic API",
// or with a billing_error.
func outOfCredits(err error) bool {
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "credit balance") || strings.Contains(text, "billing_error")
}

// askAPI goes through the Batches API, which costs half and answers within a
// day. Batches can't fall back to another model when the safety classifiers
// decline a request, so a declined one is asked again right away, at full
// price.
func askAPI(system, user string, effort anthropic.BetaOutputConfigEffort, schema map[string]any, out any) error {
	client := anthropic.NewClient(option.WithAPIKey(config.MustGet("ARXIV_WEEKLY_ANTHROPIC_API_KEY")))
	messages := []anthropic.BetaMessageParam{
		anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(user)),
	}
	outputConfig := anthropic.BetaOutputConfigParam{
		Effort: effort,
		Format: anthropic.BetaJSONOutputFormatParam{Schema: schema},
	}

	message, err := askInBatch(client, anthropic.BetaMessageBatchNewParamsRequestParams{
		Model:        model,
		MaxTokens:    maxTokens,
		System:       []anthropic.BetaTextBlockParam{{Text: system}},
		Messages:     messages,
		OutputConfig: outputConfig,
	})
	if err != nil {
		return err
	}

	if message.StopReason == anthropic.BetaStopReasonRefusal {
		log.Printf("claude: batch request declined (%s), asking again with fallback", message.StopDetails.Category)
		message, err = askNow(client, anthropic.BetaMessageNewParams{
			Model:        model,
			MaxTokens:    maxTokens,
			Betas:        []anthropic.AnthropicBeta{anthropic.AnthropicBetaServerSideFallback2026_07_01},
			Fallbacks:    anthropic.BetaFallbacksParamOfDefault(),
			System:       []anthropic.BetaTextBlockParam{{Text: system}},
			Messages:     messages,
			OutputConfig: outputConfig,
		})
		if err != nil {
			return err
		}
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

// askInBatch submits a batch of one request and waits for its answer.
func askInBatch(client anthropic.Client, params anthropic.BetaMessageBatchNewParamsRequestParams) (*anthropic.BetaMessage, error) {
	ctx := context.Background()

	batch, err := client.Beta.Messages.Batches.New(ctx, anthropic.BetaMessageBatchNewParams{
		Requests: []anthropic.BetaMessageBatchNewParamsRequest{{CustomID: "request", Params: params}},
	})
	if err != nil {
		return nil, fmt.Errorf("claude: %w", err)
	}
	log.Printf("claude: batch %s submitted", batch.ID)

	deadline := time.Now().Add(batchDeadline)
	for batch.ProcessingStatus != anthropic.BetaMessageBatchProcessingStatusEnded {
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("claude: batch %s still running after %s", batch.ID, batchDeadline)
		}
		time.Sleep(pollInterval)

		// A failed poll is not a failed batch: keep the last known state.
		polled, err := client.Beta.Messages.Batches.Get(ctx, batch.ID, anthropic.BetaMessageBatchGetParams{})
		if err != nil {
			log.Printf("claude: polling batch %s: %v", batch.ID, err)
			continue
		}
		batch = polled
	}

	results := client.Beta.Messages.Batches.ResultsStreaming(ctx, batch.ID, anthropic.BetaMessageBatchResultsParams{})
	for results.Next() {
		switch result := results.Current().Result.AsAny().(type) {
		case anthropic.BetaMessageBatchSucceededResult:
			return &result.Message, nil
		case anthropic.BetaMessageBatchErroredResult:
			return nil, fmt.Errorf("claude: batch %s: %s", batch.ID, result.Error.Error.Message)
		default:
			return nil, fmt.Errorf("claude: batch %s ended without an answer (%s)", batch.ID, results.Current().Result.Type)
		}
	}
	if err := results.Err(); err != nil {
		return nil, fmt.Errorf("claude: %w", err)
	}
	return nil, fmt.Errorf("claude: batch %s returned no results", batch.ID)
}

// askNow sends the request directly. It streams because the input is long.
func askNow(client anthropic.Client, params anthropic.BetaMessageNewParams) (*anthropic.BetaMessage, error) {
	stream := client.Beta.Messages.NewStreaming(context.Background(), params)

	message := anthropic.BetaMessage{}
	for stream.Next() {
		if err := message.Accumulate(stream.Current()); err != nil {
			return nil, fmt.Errorf("claude: %w", err)
		}
	}
	if err := stream.Err(); err != nil {
		return nil, fmt.Errorf("claude: %w", err)
	}
	return &message, nil
}
