package main

import (
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/log"
	badger "github.com/dgraph-io/badger/v4"
	"github.com/playwright-community/playwright-go"
	"github.com/trugamr/let-offer-notify/config"
)

var page playwright.Page
var db *badger.DB
var cfg *config.Config

func main() {
	// Load config
	cfg = config.NewConfig()
	if err := cfg.Load(); err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Pre-install the browser binaries
	err := playwright.Install(&playwright.RunOptions{
		Browsers: []string{"chromium"},
	})
	if err != nil {
		log.Fatalf("Failed to install Playwright: %v", err)
	}

	// Share same browser page instance to run browser minimized
	pw, err := playwright.Run()
	if err != nil {
		log.Fatalf("Failed to run Playwright: %v", err)
	}
	defer pw.Stop()

	// TODO: Handle browser close event and exit
	headless := false
	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: &headless,
	})
	if err != nil {
		log.Fatalf("Failed to launch browser: %v", err)
	}
	defer browser.Close()

	// Keep a page open to keep the browser alive
	device := pw.Devices["Desktop Chrome"]
	page, err = browser.NewPage(playwright.BrowserNewPageOptions{
		UserAgent: &device.UserAgent,
	})
	if err != nil {
		log.Fatalf("Failed to create page: %v", err)
	}
	defer page.Close()

	// Add route to intercept all requests to disable caching
	err = page.Route("**", func(r playwright.Route) {
		r.Continue()
	})
	if err != nil {
		log.Fatalf("Failed to add route: %v", err)
	}

	// Open and prepare the database
	dbopts := badger.DefaultOptions("db")
	dbopts.Logger = nil
	db, err = badger.Open(dbopts)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Create new ticker to check for new items
	ticker := time.NewTicker(45 * time.Second)
	defer ticker.Stop()

	// Using channel to first run
	immediate := make(chan struct{}, 1)
	immediate <- struct{}{}

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)

	for {
		select {
		case <-immediate:
			FetchAndProcessRSSFeed()
		case <-ticker.C:
			FetchAndProcessRSSFeed()
		case <-sc:
			log.Info("Exiting...")
			return
		}
	}

}

type RSSFeed struct {
	Channel struct {
		Title string `xml:"title"`
		Link  string `xml:"link"`
		Items []struct {
			Title       string `xml:"title"`
			Link        string `xml:"link"`
			Guid        string `xml:"guid"`
			Description string `xml:"description"`
		} `xml:"item"`
	} `xml:"channel"`
}

func FetchRSSFeed() (*RSSFeed, error) {
	resp, err := page.Goto("https://lowendtalk.com/categories/offers.rss")
	if err != nil {
		return nil, err
	}
	body, err := resp.Text()
	if err != nil {
		return nil, err
	}

	var result RSSFeed
	err = xml.Unmarshal([]byte(body), &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

func ProcessRSSFeed(result *RSSFeed) error {
	for _, item := range result.Channel.Items {
		err := db.Update(func(txn *badger.Txn) error {
			// We notify if the item is new
			notify := false

			_, err := txn.Get([]byte(item.Guid))
			if err == badger.ErrKeyNotFound {
				notify = true
				// We mark the item as seen
				err := txn.Set([]byte(item.Guid), nil)
				if err != nil {
					return err
				}
			} else {
				return err
			}

			if notify {
				log.Info("New item", "title", item.Title, "guid", item.Guid, "link", item.Link)

				// Send notification using `ntfy`
				notification := Notification{
					Title: item.Title,
					Body:  string('\u200b'), // Zero-width space
					URL:   &item.Link,
				}
				err = SendNotification(notification)
				if err != nil {
					log.Error("Failed to send notification", "err", err)
				}
			}
			return nil
		})

		if err != nil {
			return err
		}
	}
	return nil
}

func FetchAndProcessRSSFeed() {
	log.Info("Fetching RSS feed...")
	result, err := FetchRSSFeed()
	if err != nil {
		log.Fatal("Failed to fetch RSS feed", "error", err)
	}
	log.Info("Processing RSS feed...")
	err = ProcessRSSFeed(result)
	if err != nil {
		log.Fatal("Failed to process RSS feed", "error", err)
	}
	log.Info("Done!")
}

type Notification struct {
	Title string
	Body  string
	URL   *string
}

func SendNotification(n Notification) error {
	client := &http.Client{}

	req, err := http.NewRequest(http.MethodPost, cfg.Ntfy.TopicURL, strings.NewReader(n.Body))
	if err != nil {
		return err
	}

	// Set authorization header
	authorization := fmt.Sprintf("Basic %s", base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", cfg.Ntfy.Username, cfg.Ntfy.Password))))
	req.Header.Set("Authorization", authorization)

	req.Header.Set("Title", n.Title)

	// Add click action if URL is provided
	if n.URL != nil {
		req.Header.Set("Click", *n.URL)
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Check if request was successful
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to send notification: %s", resp.Status)
	}

	return nil
}
