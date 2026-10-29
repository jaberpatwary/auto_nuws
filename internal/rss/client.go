package rss

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/mmcdole/gofeed"
)

var (
	htmlTagRegex = regexp.MustCompile(`<[^>]*>`)
	imgRegex     = regexp.MustCompile(`<img[^>]+src=["']([^"']+)["']`)
)

// Item is a normalized representation of a single RSS/Atom feed entry,
// independent of the underlying feed format.
type Item struct {
	Title     string
	Content   string
	Link      string
	ImageURL  string
	Published time.Time
}

// Client wraps gofeed.Parser with robust image extraction and full text enhancement.
type Client struct {
	parser     *gofeed.Parser
	httpClient *http.Client
}

func NewClient() *Client {
	return &Client{
		parser: gofeed.NewParser(),
		httpClient: &http.Client{
			Timeout: 12 * time.Second,
		},
	}
}

// Fetch downloads and parses a single feed URL, returning normalized items with full news and images.
func (c *Client) Fetch(ctx context.Context, feedURL string) ([]Item, error) {
	ctx, cancel := context.WithTimeout(ctx, 35*time.Second)
	defer cancel()

	feed, err := c.parser.ParseURLWithContext(feedURL, ctx)
	if err != nil {
		return nil, fmt.Errorf("rss: fetch %s: %w", feedURL, err)
	}

	items := make([]Item, 0, len(feed.Items))
	for _, entry := range feed.Items {
		item := Item{
			Title: entry.Title,
			Link:  entry.Link,
		}

		if entry.Content != "" {
			item.Content = entry.Content
		} else {
			item.Content = entry.Description
		}

		item.ImageURL = c.extractImage(entry)

		// If no image found or content is not a full article (cleanLen < 1500 or fewer than 3 paragraphs),
		// fetch high-res og:image and full article content from the web page.
		if item.Link != "" && !strings.Contains(item.Link, "news.google.com") {
			cleanLen := len(strings.TrimSpace(htmlTagRegex.ReplaceAllString(item.Content, "")))
			paraCount := strings.Count(item.Content, "<p>")
			if item.ImageURL == "" || cleanLen < 1500 || paraCount < 3 {
				ogImg, fullText := c.fetchOGImageAndText(item.Link)
				if item.ImageURL == "" && ogImg != "" {
					item.ImageURL = ogImg
				}
				if len(fullText) > cleanLen && len(fullText) > 300 {
					item.Content = fullText
				}
			}
		}

		if entry.PublishedParsed != nil {
			pub := *entry.PublishedParsed
			// Cap dates: no article older than 7 days or in the future
			maxAge := time.Now().Add(-7 * 24 * time.Hour)
			if pub.Before(maxAge) {
				pub = maxAge
			}
			if pub.After(time.Now()) {
				pub = time.Now()
			}
			item.Published = pub
		} else {
			item.Published = time.Now()
		}

		items = append(items, item)
	}

	return items, nil
}

func (c *Client) extractImage(entry *gofeed.Item) string {
	if entry.Image != nil && entry.Image.URL != "" {
		return entry.Image.URL
	}
	for _, enc := range entry.Enclosures {
		if enc.URL != "" && (strings.Contains(enc.Type, "image") || strings.HasSuffix(enc.URL, ".jpg") || strings.HasSuffix(enc.URL, ".png") || strings.HasSuffix(enc.URL, ".webp") || strings.HasSuffix(enc.URL, ".jpeg")) {
			return enc.URL
		}
	}
	if len(entry.Enclosures) > 0 && entry.Enclosures[0].URL != "" {
		return entry.Enclosures[0].URL
	}
	if media, ok := entry.Extensions["media"]; ok {
		if t, ok := media["thumbnail"]; ok && len(t) > 0 {
			if u := t[0].Attrs["url"]; u != "" {
				return u
			}
		}
		if cont, ok := media["content"]; ok && len(cont) > 0 {
			if u := cont[0].Attrs["url"]; u != "" {
				return u
			}
		}
	}
	if m := imgRegex.FindStringSubmatch(entry.Description); len(m) > 1 {
		return m[1]
	}
	if m := imgRegex.FindStringSubmatch(entry.Content); len(m) > 1 {
		return m[1]
	}
	return ""
}

func (c *Client) fetchOGImageAndText(pageURL string) (string, string) {
	req, err := http.NewRequest("GET", pageURL, nil)
	if err != nil {
		return "", ""
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "bn-BD,bn;q=0.9,en-US;q=0.8,en;q=0.7")

	resp, err := c.httpClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return "", ""
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return "", ""
	}

	// 1. Image extraction: og:image -> twitter:image -> itemprop -> first article img
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

	// 2. Full text extraction with BD news CSS selectors
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

