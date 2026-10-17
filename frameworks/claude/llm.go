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
)

// A direct answer takes minutes; a batch fallback inside llm can take up to a day.
const cliTimeout = 35 * time.Minute
const llmDeadline = 26 * time.Hour

// Room for the longest answer (the picks with their reasons).
const maxTokens = 32000

// askLLM gets the JSON answer to one request through an external `llm` command (see the README for the interface it
// must follow): the command decides which provider answers and enforces the JSON schema. LLM_BIN says where the command
// is; without it, `llm` is looked up in the PATH.
func askLLM(system, user, effort string, schema map[string]any, out any) error {
	schemaJSON, err := json.Marshal(schema)
	if err != nil {
		return err
	}
	// The system prompt carries the whole interest profile, so it goes in a file rather than on the command line.
	systemFile, err := os.CreateTemp("", "arxiv-weekly-system-*.txt")
	if err != nil {
		return err
	}
	defer os.Remove(systemFile.Name())
	if _, err := systemFile.WriteString(system); err != nil {
		systemFile.Close()
		return err
	}
	systemFile.Close()

	bin := os.Getenv("LLM_BIN")
	if bin == "" {
		if bin, err = exec.LookPath("llm"); err != nil {
			return fmt.Errorf("llm not found: set LLM_BIN or put llm in the PATH")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), llmDeadline)
	defer cancel()
	command := exec.CommandContext(ctx, bin,
		"--sistema", "@"+systemFile.Name(), "--nivel", "mejor", "--esfuerzo", effort, "--sin-apuro",
		"--esquema", string(schemaJSON), "--max-tokens", fmt.Sprint(maxTokens),
		"--timeout", fmt.Sprint(int(cliTimeout.Seconds())))
	command.Stdin = strings.NewReader(user)
	var stderr bytes.Buffer
	command.Stderr = &stderr

	output, err := command.Output()
	if err != nil {
		return fmt.Errorf("llm: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	if err := json.Unmarshal(output, out); err != nil {
		return fmt.Errorf("llm: unreadable answer: %w", err)
	}
	log.Printf("llm: answered (%d bytes)", len(output))
	return nil
}
