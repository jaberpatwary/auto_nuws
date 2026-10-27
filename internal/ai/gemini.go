package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const geminiEndpointTemplate = "https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s"

// GeminiClient calls Google's Gemini generateContent endpoint.
type GeminiClient struct {
	apiKey string
	model  string
	http   *http.Client
}

func NewGeminiClient(apiKey, model string) *GeminiClient {
	return &GeminiClient{
		apiKey: apiKey,
		model:  model,
		http:   &http.Client{Timeout: 30 * time.Second},
	}
}

func (g *GeminiClient) Name() string  { return "gemini" }
func (g *GeminiClient) Model() string { return g.model }

type geminiRequest struct {
	Contents []geminiContent `json:"contents"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiResponse struct {
	Candidates []struct {
		Content geminiContent `json:"content"`
	} `json:"candidates"`
	UsageMetadata struct {
		TotalTokenCount int `json:"totalTokenCount"`
	} `json:"usageMetadata"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (g *GeminiClient) Summarize(ctx context.Context, title, content string) (string, int, error) {
	if g.apiKey == "" {
		return "", 0, fmt.Errorf("gemini: GEMINI_API_KEY is not set")
	}

	prompt := buildSummaryPrompt(title, content)

	reqBody := geminiRequest{
		Contents: []geminiContent{
			{Parts: []geminiPart{{Text: prompt}}},
		},
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", 0, fmt.Errorf("gemini: marshal request: %w", err)
	}

	url := fmt.Sprintf(geminiEndpointTemplate, g.model, g.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", 0, fmt.Errorf("gemini: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.http.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("gemini: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, fmt.Errorf("gemini: read response: %w", err)
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		return "", 0, fmt.Errorf("gemini: rate limited (429)")
	}
	if resp.StatusCode >= 400 {
		return "", 0, fmt.Errorf("gemini: http %d: %s", resp.StatusCode, string(body))
	}

	var parsed geminiResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", 0, fmt.Errorf("gemini: unmarshal response: %w", err)
	}
	if parsed.Error != nil {
		return "", 0, fmt.Errorf("gemini: api error: %s", parsed.Error.Message)
	}
	if len(parsed.Candidates) == 0 || len(parsed.Candidates[0].Content.Parts) == 0 {
		return "", 0, fmt.Errorf("gemini: empty candidates in response")
	}

	return parsed.Candidates[0].Content.Parts[0].Text, parsed.UsageMetadata.TotalTokenCount, nil
}
