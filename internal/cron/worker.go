package cron

import (
	"context"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"bangla-news-aggregator/internal/ai"
	"bangla-news-aggregator/internal/config"
	"bangla-news-aggregator/internal/domain"
	"bangla-news-aggregator/internal/repository"
	"bangla-news-aggregator/internal/rss"
)

var htmlTagRegex = regexp.MustCompile(`<[^>]*>`)

// Worker runs the periodic feed-fetch → dedupe → summarize → publish pipeline.
type Worker struct {
	cfg         *config.Config
	feedRepo    repository.FeedRepository
	articleRepo repository.ArticleRepository
	usageRepo   repository.AIUsageRepository
	rssClient   *rss.Client
	aiClient    *ai.Client
}

func NewWorker(
	cfg *config.Config,
	feedRepo repository.FeedRepository,
	articleRepo repository.ArticleRepository,
	usageRepo repository.AIUsageRepository,
) *Worker {
	return &Worker{
		cfg:         cfg,
		feedRepo:    feedRepo,
		articleRepo: articleRepo,
		usageRepo:   usageRepo,
		rssClient:   rss.NewClient(),
		aiClient:    ai.NewClient(cfg),
	}
}

// Start blocks and runs the pipeline immediately, then on every tick of
// cfg.CronInterval, until the context is cancelled.
func (w *Worker) Start(ctx context.Context) {
	log.Printf("[cron] worker started, interval=%s, workers=%d", w.cfg.CronInterval, w.cfg.FeedWorkerCount)

	w.RunOnce(ctx)

	ticker := time.NewTicker(w.cfg.CronInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("[cron] worker stopped")
			return
		case <-ticker.C:
			w.RunOnce(ctx)
		}
	}
}

// RunOnce executes a single pass: load active feeds, fetch them in
// parallel with a bounded worker pool, dedupe by source_url, summarize
// with AI (Groq → Gemini fallback), and save as published.
func (w *Worker) RunOnce(ctx context.Context) {
	start := time.Now()
	log.Println("[cron] run started")

	feeds, err := w.feedRepo.ListActive()
	if err != nil {
		log.Printf("[cron] failed to load active feeds: %v", err)
		return
	}
	if len(feeds) == 0 {
		log.Println("[cron] no active feeds configured, skipping run")
		return
	}

	feedCh := make(chan domain.FeedSource)
	var wg sync.WaitGroup

	var (
		mu        sync.Mutex
		processed int
		saved     int
		skipped   int
		failed    int
	)

	workerCount := w.cfg.FeedWorkerCount
	if workerCount < 1 {
		workerCount = 1
	}

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for feed := range feedCh {
				p, s, sk, f := w.processFeed(ctx, feed)
				mu.Lock()
				processed += p
				saved += s
				skipped += sk
				failed += f
				mu.Unlock()
			}
		}(i)
	}

	for _, f := range feeds {
		feedCh <- f
	}
	close(feedCh)
	wg.Wait()

	log.Printf(
		"[cron] run finished in %s: feeds=%d processed=%d saved=%d skipped(dupe)=%d failed=%d",
		time.Since(start), len(feeds), processed, saved, skipped, failed,
	)
}

// processFeed fetches one feed, dedupes its items, and summarizes +
// saves each new item. Returns (processed, saved, skipped, failed) counts.
func (w *Worker) processFeed(ctx context.Context, feed domain.FeedSource) (processed, saved, skipped, failed int) {
	items, err := w.rssClient.Fetch(ctx, feed.URL)
	if recErr := w.feedRepo.RecordFetch(feed.ID, err); recErr != nil {
		log.Printf("[cron] could not record fetch status for feed %q: %v", feed.Name, recErr)
	}
	if err != nil {
		log.Printf("[cron] feed %q fetch failed: %v", feed.Name, err)
		return 0, 0, 0, 1
	}

	for _, item := range items {
		processed++

		if item.Link == "" || strings.TrimSpace(item.Title) == "" {
			continue
		}

		exists, err := w.articleRepo.ExistsBySourceURL(item.Link)
		if err != nil {
			log.Printf("[cron] dedupe check failed for %q: %v", item.Link, err)
			failed++
			continue
		}
		if exists {
			skipped++
			continue
		}

		result, attempts := w.aiClient.Summarize(ctx, item.Title, item.Content)
		for _, attempt := range attempts {
			w.logAIUsage(attempt)
		}

		summary := result.Summary
		if result.Err != nil || summary == "" {
			log.Printf("[cron] AI summarization failed for %q, saving without summary: %v", item.Title, result.Err)
			cleaned := htmlTagRegex.ReplaceAllString(item.Content, " ")
			cleaned = strings.Join(strings.Fields(cleaned), " ")
			summary = truncate(cleaned, 280)
		}

		article := &domain.Article{
			Title:       item.Title,
			Content:     item.Content,
			Summary:     summary,
			SourceURL:   item.Link,
			SourceName:  feed.Name,
			ImageURL:    item.ImageURL,
			Category:    feed.Category,
			Status:      "published",
			PublishedAt: item.Published,
		}

		if err := w.articleRepo.Create(article); err != nil {
			log.Printf("[cron] failed to save article %q: %v", item.Title, err)
			failed++
			continue
		}

		saved++
	}

	return processed, saved, skipped, failed
}

func (w *Worker) logAIUsage(r ai.Result) {
	usage := &domain.AIUsage{
		Provider:   r.Provider,
		Model:      r.Model,
		TokensUsed: r.TokensUsed,
		Success:    r.Err == nil && r.Summary != "",
		DurationMs: r.Duration.Milliseconds(),
	}
	if r.Err != nil {
		usage.ErrorMsg = r.Err.Error()
	}
	if err := w.usageRepo.Create(usage); err != nil {
		log.Printf("[cron] failed to log ai usage: %v", err)
	}
}

func truncate(s string, max int) string {
	if max <= 0 || s == "" {
		return ""
	}
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max]) + "..."
}
