package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"arxiv-weekly/config"
)

// The CLI answers right away, so a run this long is stuck.
const cliDeadline = 30 * time.Minute

// askCLI gets the JSON answer to one request from the claude CLI and decodes
// it into out. The CLI logs in with a `claude setup-token` token, since cron
// can't reach the login it keeps in the keychain, and runs without tools: all
// it has to do is answer.
func askCLI(system, user, effort string, schema map[string]any, out any) error {
	schemaJSON, err := json.Marshal(schema)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), cliDeadline)
	defer cancel()
	command := exec.CommandContext(ctx, "claude", "--print",
		"--tools", "", "--strict-mcp-config", "--no-session-persistence",
		"--model", model, "--effort", effort,
		"--system-prompt", system,
		"--output-format", "json", "--json-schema", string(schemaJSON))
	command.Stdin = strings.NewReader(user)
	command.Env = append(os.Environ(),
		"CLAUDE_CODE_OAUTH_TOKEN="+config.MustGet("ARXIV_WEEKLY_CLAUDE_OAUTH_TOKEN"))
	var stderr bytes.Buffer
	command.Stderr = &stderr

	var result struct {
		IsError          bool            `json:"is_error"`
		Result           string          `json:"result"`
		StructuredOutput json.RawMessage `json:"structured_output"`
		Usage            struct {
			OutputTokens int64 `json:"output_tokens"`
		} `json:"usage"`
	}
	// A failed run explains itself in the result when it got as far as
	// printing one, and on stderr when it didn't.
	output, runErr := command.Output()
	parseErr := json.Unmarshal(output, &result)
	if runErr != nil || result.IsError {
		detail := result.Result
		if detail == "" {
			detail = strings.TrimSpace(stderr.String())
		}
		if runErr != nil {
			return fmt.Errorf("claude CLI: %w: %s", runErr, detail)
		}
		return fmt.Errorf("claude CLI: %s", detail)
	}
	if parseErr != nil {
		return fmt.Errorf("claude CLI: unreadable output: %w", parseErr)
	}
	log.Printf("claude CLI: model %s, %d output tokens", model, result.Usage.OutputTokens)

	if err := json.Unmarshal(result.StructuredOutput, out); err != nil {
		return fmt.Errorf("claude CLI: unreadable answer: %w", err)
	}
	return nil
}
