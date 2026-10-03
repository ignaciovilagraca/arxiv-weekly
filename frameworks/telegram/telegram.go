package telegram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"arxiv-weekly/config"
)

const telegramApi = "https://api.telegram.org/bot%v/sendMessage"

// SendMessage sends an HTML-formatted message to the configured chat.
func SendMessage(html string) error {
	url := fmt.Sprintf(telegramApi, config.MustGet("ARXIV_WEEKLY_TELEGRAM_TOKEN"))
	body, err := json.Marshal(map[string]any{
		"chat_id":              config.MustGet("ARXIV_WEEKLY_TELEGRAM_CHAT_ID"),
		"text":                 html,
		"parse_mode":           "HTML",
		"link_preview_options": map[string]bool{"is_disabled": true},
	})
	if err != nil {
		return err
	}

	res, err := http.Post(url, "application/json", bytes.NewBuffer(body))
	if err != nil {
		// The error carries the URL, and the URL carries the bot token.
		return fmt.Errorf("telegram: request failed")
	}
	defer res.Body.Close()

	var result struct {
		Ok          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return fmt.Errorf("telegram: status %d", res.StatusCode)
	}
	if !result.Ok {
		return fmt.Errorf("telegram: %s", result.Description)
	}

	return nil
}
