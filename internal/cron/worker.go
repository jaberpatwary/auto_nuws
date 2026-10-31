package cron

import (
	"context"
	"errors"
	"html"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"
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

	// Anchor the recurring schedule to worker startup. Starting the ticker only
	// after RunOnce makes every slow fetch permanently push the next poll later.
	ticker := time.NewTicker(w.cfg.CronInterval)
	defer ticker.Stop()

	w.RunOnce(ctx)

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
		"[cron] run finished in %s: feeds=%d processed=%d saved=%d skipped=%d failed=%d",
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
		exists, err = w.articleRepo.ExistsByTitleAndDate(item.Title, item.Published)
		if err != nil {
			log.Printf("[cron] headline dedupe check failed for %q: %v", item.Title, err)
			failed++
			continue
		}
		if exists {
			skipped++
			continue
		}

		visibleContent := html.UnescapeString(htmlTagRegex.ReplaceAllString(item.Content, " "))
		visibleRunes := utf8.RuneCountInString(strings.TrimSpace(visibleContent))
		if item.ImageURL == "" || visibleRunes < 600 {
			log.Printf("[cron] skipped incomplete story %q (body=%d chars, image=%t)", item.Title, visibleRunes, item.ImageURL != "")
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

		sourceName := feed.Name
		if item.PublisherName != "" {
			sourceName = item.PublisherName
		}
		article := &domain.Article{
			Title:       item.Title,
			Content:     item.Content,
			Summary:     summary,
			SourceURL:   item.Link,
			SourceName:  sourceName,
			ImageURL:    item.ImageURL,
			Category:    classifyItemCategory(item.Title, item.Categories, feed.Category),
			Status:      "published",
			PublishedAt: item.Published,
		}

		if err := w.articleRepo.Create(article); err != nil {
			if errors.Is(err, repository.ErrDuplicateArticle) {
				skipped++
				continue
			}
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

var (
	sportsKeywords = []string{
		"ক্রিকেট", "ফুটবল", "খেলাধুলা", "ম্যাচ", "মেসি", "রোনালদো", "সাকিব", "তামিম", "শান্ত",
		"মুশফিক", "উইকেট", "রান", "সেঞ্চুরি", "ফিফটি", "গোল", "টাইব্রেকার", "ব্যালন ডি’অর",
		"বিপিএল", "আইপিএল", "বিশ্বকাপ", "সিরিজ", "টেস্ট", "ওয়ানডে", "টি-টোয়েন্টি", "টি২০",
		"অলরাউন্ডার", "বোলার", "ব্যাটার", "আম্পায়ার", "ফিফা", "আইসিসি", "লা লিগা",
		"চ্যাম্পিয়ন্স লিগ", "প্রিমিয়ার লিগ", "নেইমার", "রিয়াল মাদ্রিদ", "বার্সেলোনা",
	}

	businessKeywords = []string{
		"ব্যবসা", "বাণিজ্য", "অর্থনীতি", "ব্যাংক", "টাকা", "ডলার", "শেয়ারবাজার", "ডিএসই",
		"মুদ্রাস্ফীতি", "মূল্যস্ফীতি", "রাজস্ব", "বাজেট", "রপ্তানি", "আমদানি", "ঋণ", "সুদ",
		"জিডিপি", "পুঁজিবাজার", "বিনিয়োগ", "আইএমএফ", "বাণিজ্য ঘাটতি", "রেমিট্যান্স", "কাস্টমস",
		"ভ্যাট", "স্বর্ণের দাম", "তেলের দাম", "পে স্কেল", "গ্রেডের ক্রম", "বাড়ি ভাড়া", "শিল্প কারখানা",
	}

	techKeywords = []string{
		"প্রযুক্তি", "বিজ্ঞান", "স্মার্টফোন", "মোবাইল", "কম্পিউটার", "ল্যাপটপ", "ইন্টারনেট",
		"ওয়াই-ফাই", "সফটওয়্যার", "অ্যাপ", "গুগল", "ফেসবুক", "মেটা", "মাইক্রোসফট", "অ্যাপল",
		"এআই", "কৃত্রিম বুদ্ধিমত্তা", "সাইবার", "হ্যাক", "রোবট", "মহাকাশ", "নাসা", "ইলন মাস্ক",
		"সোশ্যাল মিডিয়া", "ইউটিউব", "ইনস্টাগ্রাম", "টুইটার", "চ্যাটজিপিটি", "টেলিকম", "ফোরজি", "ফাইভজি",
	}

	entertainmentKeywords = []string{
		"বিনোদন", "চলচ্চিত্র", "সিনেমা", "নাটক", "অভিনেতা", "অভিনেত্রী", "গায়ক", "গায়িকা",
		"নায়ক", "নায়িকা", "বলিউড", "হলিউড", "ঢালিউড", "গান", "মিউজিক", "শাকিব খান",
		"শাহরুখ", "সালমান", "আমির", "টাইগার শ্রফ", "অস্কার", "ফিল্ম", "কনসার্ট", "তারকা", "শুটিং",
		"পরিচালক", "সেলিব্রিটি", "হাল ফ্যাশন", "অপু বিশ্বাস", "বুবলী", "মেহজাবীন",
	}

	intlKeywords = []string{
		"আন্তর্জাতিক", "যুক্তরাষ্ট্র", "আমেরিকা", "ইসরায়েল", "ফিলিস্তিন", "গাজা", "লেবানন",
		"ইরান", "রাশিয়া", "ইউক্রেন", "পুতিন", "বাইডেন", "ট্রাম্প", "চীন", "ভারত", "পাকিস্তান",
		"জাতিসংঘ", "নেতানিয়াহু", "মধ্যপ্রাচ্য", "সিরিয়া", "হোয়াইট হাউস", "ওয়াশিংটন", "হিজবুল্লাহ",
		"হামাস", "ন্যাটো", "বেইজিং", "মস্কো", "তেহরান",
	}

	bangladeshKeywords = []string{
		"বাংলাদেশ", "ঢাকা", "চট্টগ্রাম", "সিলেট", "রাজশাহী", "খুলনা", "বরিশাল", "রংপুর",
		"ময়মনসিংহ", "উপদেষ্টা", "ইউনূস", "অন্তর্বর্তী", "নির্বাচন", "বিএনপি", "আওয়ামী লীগ",
		"জামায়াত", "ছাত্রদল", "শিবির", "বৈষম্যবিরোধী", "পুলিশ", "আদালত", "হাইকোর্ট",
		"আইনশৃঙ্খলা", "উপজেলা", "জেলা", "থানা", "গ্রেফতার", "মামলা", "সচিবালয়", "ঘূর্ণিঝড়",
		"সমুদ্রবন্দর", "কারাগার", "আসামি", "খালেদা জিয়া", "তারেক রহমান", "পল্টন", "সিপিবি",
	}
)

func countMatches(text string, keywords []string) int {
	score := 0
	lower := strings.ToLower(text)
	for _, kw := range keywords {
		// These short words commonly occur inside unrelated Bengali words:
		// "খেলা" matches "খেলাপি" (loan defaulter), while "রান" matches "ইরান".
		if kw == "\u0996\u09c7\u09b2\u09be" || kw == "\u099f\u09be\u0995\u09be" || kw == "\u09aa\u09b0\u09bf\u099a\u09be\u09b2\u0995" {
			continue
		}
		// A passing mention of a social platform does not make a story technology news.
		if kw == "\u09ab\u09c7\u09b8\u09ac\u09c1\u0995" || kw == "\u099f\u09c1\u0987\u099f\u09be\u09b0" || kw == "\u0987\u09a8\u09b8\u09cd\u099f\u09be\u0997\u09cd\u09b0\u09be\u09ae" {
			continue
		}
		if matchesKeyword(lower, kw) {
			score++
		}
	}
	return score
}

func matchesKeyword(text, keyword string) bool {
	if strings.ContainsAny(keyword, " \t") {
		return strings.Contains(text, keyword)
	}
	for _, token := range strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsMark(r) && !unicode.IsNumber(r)
	}) {
		if token == keyword || strings.HasPrefix(token, keyword) {
			return true
		}
	}
	return false
}

// detectCategory inspects the article text and maps it into one of:
// "sports", "business", "technology", "entertainment", "international", "bangladesh"
func detectCategory(title, content, defaultCategory string) string {
	// Publisher pages often contain related-story lists and navigation text.
	// Scoring the body let those unrelated terms put stories in the wrong menu.
	// Headlines provide a much cleaner signal for category placement.
	text := title
	type catScore struct {
		name  string
		score int
	}

	scores := []catScore{
		{"sports", countMatches(text, sportsKeywords) * 3},
		{"business", countMatches(text, businessKeywords) * 2},
		{"technology", countMatches(text, techKeywords) * 3},
		{"entertainment", countMatches(text, entertainmentKeywords) * 3},
		{"international", countMatches(text, intlKeywords) * 2},
		{"bangladesh", countMatches(text, bangladeshKeywords) * 2},
	}

	bestCat := ""
	bestScore := 0
	for _, s := range scores {
		if s.score > bestScore {
			bestScore = s.score
			bestCat = s.name
		}
	}

	if bestScore >= 2 {
		return bestCat
	}

	defaultCategory = strings.ToLower(strings.TrimSpace(defaultCategory))
	switch defaultCategory {
	case "bangladesh", "international", "business", "sports", "technology", "entertainment", "general":
		return defaultCategory
	}
	return "general"
}

// ClassifyCategory exposes the same category rules to one-off data backfills.
func ClassifyCategory(title, content, defaultCategory string) string {
	return detectCategory(title, content, defaultCategory)
}

func classifyItemCategory(title string, feedCategories []string, defaultCategory string) string {
	hasBangladeshLabel := false
	for _, raw := range feedCategories {
		label := strings.ToLower(strings.TrimSpace(raw))
		switch {
		case strings.Contains(label, "sport"), strings.Contains(label, "খেলা"), strings.Contains(label, "ক্রিকেট"), strings.Contains(label, "ফুটবল"):
			return "sports"
		case strings.Contains(label, "business"), strings.Contains(label, "economy"), strings.Contains(label, "ব্যবসা"), strings.Contains(label, "বাণিজ্য"), strings.Contains(label, "অর্থনীতি"):
			return "business"
		case strings.Contains(label, "technology"), strings.Contains(label, "tech"), strings.Contains(label, "প্রযুক্তি"), strings.Contains(label, "বিজ্ঞান"):
			return "technology"
		case strings.Contains(label, "entertainment"), strings.Contains(label, "বিনোদন"), strings.Contains(label, "বলিউড"), strings.Contains(label, "ঢালিউড"):
			return "entertainment"
		case strings.Contains(label, "international"), strings.Contains(label, "world"), strings.Contains(label, "আন্তর্জাতিক"), strings.Contains(label, "বিশ্ব"), strings.Contains(label, "ভারত"):
			return "international"
		case strings.Contains(label, "bangladesh"), strings.Contains(label, "বাংলাদেশ"), strings.Contains(label, "জাতীয়"), strings.Contains(label, "রাজনীতি"):
			hasBangladeshLabel = true
		}
	}
	category := detectCategory(title, "", defaultCategory)
	if category != "general" || !hasBangladeshLabel {
		return category
	}
	return "bangladesh"
}
