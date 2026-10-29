package main

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"

	"bangla-news-aggregator/internal/config"
	"bangla-news-aggregator/internal/domain"
	"bangla-news-aggregator/internal/repository"
)

func fetchOGData(client *http.Client, pageURL string) (string, string) {
	req, err := http.NewRequest("GET", pageURL, nil)
	if err != nil {
		return "", ""
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "bn-BD,bn;q=0.9,en-US;q=0.8,en;q=0.7")

	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return "", ""
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return "", ""
	}

	ogImg, _ := doc.Find(`meta[property="og:image"]`).Attr("content")
	if ogImg == "" {
		ogImg, _ = doc.Find(`meta[name="twitter:image"]`).Attr("content")
	}
	if ogImg == "" {
		ogImg, _ = doc.Find(`meta[itemprop="image"]`).Attr("content")
	}
	if ogImg == "" {
		ogImg, _ = doc.Find(`meta[name="thumbnail"]`).Attr("content")
	}
	if ogImg == "" {
		ogImg, _ = doc.Find(".featured-image img, figure img, article img, .lead-image img, .detail-box img").First().Attr("src")
	}

	// Resolve relative image URL if needed
	if ogImg != "" && strings.HasPrefix(ogImg, "/") && !strings.HasPrefix(ogImg, "//") {
		if strings.HasPrefix(pageURL, "http") {
			parts := strings.Split(pageURL, "/")
			if len(parts) >= 3 {
				domain := parts[0] + "//" + parts[2]
				ogImg = domain + ogImg
			}
		}
	}

	seenParas := make(map[string]bool)
	var paras []string
	selectors := ".details-body p, .details-main p, .dNewsDesc p, .entry-content p, .post-content p, .detail-box p, .details-text p, .content-details p, .news-details p, .jw-detail p, [itemprop='articleBody'] p, .story-body p, .article-body p, .story-element p, article p, main p, [role='main'] p"
	doc.Find(selectors).Each(func(i int, s *goquery.Selection) {
		t := strings.TrimSpace(s.Text())
		if len([]rune(t)) > 25 && !seenParas[t] {
			if strings.HasPrefix(t, "ছবির উৎস") || strings.HasPrefix(t, "ছবি ক্যাপশন") || strings.Contains(t, "ALL RIGHTS RESERVED") || strings.Contains(t, "বিজ্ঞাপন :") || strings.Contains(t, "Blazing-fast frontend") {
				return
			}
			seenParas[t] = true
			paras = append(paras, "<p>"+t+"</p>")
		}
	})

	fullText := strings.Join(paras, "\n")
	return ogImg, fullText
}

func main() {
	cfg := config.Load()
	db, err := repository.NewPostgresDB(cfg)
	if err != nil {
		log.Fatal(err)
	}

	var articles []domain.Article
	// Find articles without image or where content is shorter than 1500 chars (not full news)
	db.Where("(image_url = '' OR image_url IS NULL OR length(content) < 1500) AND source_url NOT LIKE '%news.google.com%'").Find(&articles)
	fmt.Printf("Found %d articles to enhance with images & full text...\n", len(articles))

	client := &http.Client{Timeout: 12 * time.Second}
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)

	updatedImg := 0
	updatedContent := 0
	var mu sync.Mutex

	for _, a := range articles {
		wg.Add(1)
		sem <- struct{}{}
		go func(art domain.Article) {
			defer wg.Done()
			defer func() { <-sem }()

			img, fullText := fetchOGData(client, art.SourceURL)
			save := false
			if art.ImageURL == "" && img != "" {
				art.ImageURL = img
				save = true
				mu.Lock()
				updatedImg++
				mu.Unlock()
			}
			if len(fullText) > len(art.Content) && len(fullText) > 400 {
				art.Content = fullText
				save = true
				mu.Lock()
				updatedContent++
				mu.Unlock()
			}
			if save {
				db.Save(&art)
				fmt.Printf("✅ Enhanced: [%s] %s | Img: %v | ContentLen: %d\n", art.SourceName, art.Title, art.ImageURL != "", len(art.Content))
			}
		}(a)
	}
	wg.Wait()

	fmt.Printf("\n🎉 Finished backfill! Updated images: %d, Updated full text: %d\n", updatedImg, updatedContent)
}
