package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"site/storage"
	"time"
)

type ModeratorConfig struct {
	Store    *storage.Store
	JobQueue <-chan int
	URL      string
	Model    string
}

type Moderator struct {
	config ModeratorConfig
}

func NewModerator(config ModeratorConfig) *Moderator {
	return &Moderator{
		config: config,
	}
}

func (m *Moderator) Start(ctx context.Context) {
	for {
		select {
		case postID := <-m.config.JobQueue:
			text, err := m.config.Store.GetEntry(postID)
			if err != nil {
				slog.Error("Error retrieving Post from Store", "error", err)
			}
			isApproved, err := m.isPostApproved(text.Content)
			if err != nil {
				slog.Error("Failed to Moderate Post in Textservice", "error", err)
			}
			if err == nil && !isApproved {
				if err = m.config.Store.RemoveEntry(postID); err != nil {
					slog.Error("Couldn't delete Post", "error", err)
				}
			}
		case <-ctx.Done():
			slog.Info("Moderator shutting down...")
			return
		}
	}
}

func (m *Moderator) isPostApproved(text string) (bool, error) {
	req := struct {
		Model  string `json:"model"`
		Prompt string `json:"prompt"`
	}{
		Model:  m.config.Model,
		Prompt: text,
	}

	body, err := json.Marshal(&req)
	if err != nil {
		return false, fmt.Errorf("Failed to encode JSON: %w", err)
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Post(m.config.URL, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return false, fmt.Errorf("Couldn't get moderation response: %w", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("Couldn't get moderation response. Statuscode: %d", resp.StatusCode)
	}

	type AIResult struct {
		IsApproved bool   `json:"is_approved"`
		Reason     string `json:"reason"`
	}

	var aiResult AIResult
	if err := json.NewDecoder(resp.Body).Decode(&aiResult); err != nil {
		return false, fmt.Errorf("Failed to decode JSON response: %w", err)
	}

	slog.Info("Moderation complete", "approved", aiResult.IsApproved, "reason", aiResult.Reason)

	return aiResult.IsApproved, nil
}
