package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"time"

	"site/helper"
	"site/storage"
)

type TextService struct {
	URL                string `json:"-"`
	Model              string `json:"model"`
	Prompt             string `json:"prompt"`
	SystemInstructions string `json:"system_instructions"`
}

type TextResponse struct {
	Text string `json:"text"`
}

type AutoPosterConfig struct {
	Store       *storage.Store
	MinInterval time.Duration
	MaxInterval time.Duration
	URL         string
	Model       string
	Prompts     []string
}

type AutoPoster struct {
	config AutoPosterConfig
}

func NewAutoPoster(config AutoPosterConfig) *AutoPoster {
	return &AutoPoster{
		config: config,
	}
}

func (p *AutoPoster) getText() (string, error) {
	var textService TextService
	textService.URL = p.config.URL
	textService.Model = p.config.Model
	textService.Prompt = p.config.Prompts[rand.IntN(len(p.config.Prompts))]

	body, err := json.Marshal(&textService)
	if err != nil {
		return "", fmt.Errorf("Failed to encode JSON: %w", err)
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Post(textService.URL, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return "", fmt.Errorf("Couldn't get Text from TextService: %w", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Couldn't get Text from TextService. Statuscode: %d", resp.StatusCode)
	}

	var apiResponse TextResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResponse); err != nil {
		return "", fmt.Errorf("Failed to decode JSON response: %w", err)
	}

	return apiResponse.Text, nil
}

// Start runs the periodic posting loop until the context is canceled
func (p *AutoPoster) Start(ctx context.Context) {
	randomTicker := helper.NewRandomTicker(p.config.MinInterval, p.config.MaxInterval)
	defer randomTicker.Stop()

	for {
		select {
		case <-randomTicker.C:
			newText, err := p.getText()
			if err != nil {
				slog.Error("AutoPoster failed to get Post from TextService", "error", err)
				continue
			}
			if _, err := p.config.Store.AddEntry(newText, "system-autoposter-worker"); err != nil {
				slog.Error("AutoPoster failed to add entry", "error", err)
			} else {
				slog.Info("AutoPoster created a new entry")
			}
		case <-ctx.Done():
			slog.Info("AutoPoster shutting down...")
			return
		}
	}
}
