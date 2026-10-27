package rss

import (
	"context"
	"fmt"
	"time"

	"github.com/mmcdole/gofeed"
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

// Client wraps gofeed.Parser with a timeout-bound fetch method.
type Client struct {
	parser *gofeed.Parser
}

func NewClient() *Client {
	return &Client{parser: gofeed.NewParser()}
}

// Fetch downloads and parses a single feed URL, returning normalized items.
func (c *Client) Fetch(ctx context.Context, feedURL string) ([]Item, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
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

		if entry.Image != nil {
			item.ImageURL = entry.Image.URL
		} else if len(entry.Enclosures) > 0 {
			item.ImageURL = entry.Enclosures[0].URL
		}

		if entry.PublishedParsed != nil {
			item.Published = *entry.PublishedParsed
		} else {
			item.Published = time.Now()
		}

		items = append(items, item)
	}

	return items, nil
}
