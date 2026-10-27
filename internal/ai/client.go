package ai

import (
	"context"
	"errors"
	"log"
	"time"

	"bangla-news-aggregator/internal/config"
)

// Result carries the outcome of a single summarization call, including
// metadata used for ai_usage logging regardless of success or failure.
type Result struct {
	Provider   string
	Model      string
	Summary    string
	TokensUsed int
	Duration   time.Duration
	Err        error
}

// Summarizer is implemented by each AI provider (Groq, Gemini, ...).
type Summarizer interface {
	Name() string
	Model() string
	Summarize(ctx context.Context, title, content string) (summary string, tokensUsed int, err error)
}

// Client orchestrates summarization with automatic fallback: it tries the
// primary provider first (Groq) and, on any error (including rate limits),
// falls back to the secondary provider (Gemini).
type Client struct {
	primary   Summarizer
	secondary Summarizer
}

// NewClient builds the fallback-aware AI client from configuration.
func NewClient(cfg *config.Config) *Client {
	return &Client{
		primary:   NewGroqClient(cfg.GroqAPIKey, cfg.GroqModel),
		secondary: NewGeminiClient(cfg.GeminiAPIKey, cfg.GeminiModel),
	}
}

// Summarize tries the primary provider, then the secondary, returning the
// first successful Result. Both attempts are returned to the caller via the
// usage channel-like return so each can be logged to ai_usage, including
// failures.
func (c *Client) Summarize(ctx context.Context, title, content string) (Result, []Result) {
	var attempts []Result

	start := time.Now()
	summary, tokens, err := c.primary.Summarize(ctx, title, content)
	primaryResult := Result{
		Provider:   c.primary.Name(),
		Model:      c.primary.Model(),
		Summary:    summary,
		TokensUsed: tokens,
		Duration:   time.Since(start),
		Err:        err,
	}
	attempts = append(attempts, primaryResult)

	if err == nil && summary != "" {
		return primaryResult, attempts
	}

	log.Printf("[ai] primary provider %q failed (%v), falling back to %q", c.primary.Name(), err, c.secondary.Name())

	start = time.Now()
	summary, tokens, err = c.secondary.Summarize(ctx, title, content)
	secondaryResult := Result{
		Provider:   c.secondary.Name(),
		Model:      c.secondary.Model(),
		Summary:    summary,
		TokensUsed: tokens,
		Duration:   time.Since(start),
		Err:        err,
	}
	attempts = append(attempts, secondaryResult)

	if err == nil && summary != "" {
		return secondaryResult, attempts
	}

	return Result{Err: errors.New("both AI providers failed")}, attempts
}
