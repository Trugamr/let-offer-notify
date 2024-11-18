package main

import (
	"encoding/xml"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/charmbracelet/log"
	badger "github.com/dgraph-io/badger/v4"
	"github.com/playwright-community/playwright-go"
)

var page playwright.Page
var db *badger.DB

func main() {
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

	headless := false
	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: &headless,
	})
	if err != nil {
		log.Fatalf("Failed to launch browser: %v", err)
	}
	defer browser.Close()

	// Keep a page open to keep the browser alive
	page, err = browser.NewPage()
	if err != nil {
		log.Fatalf("Failed to create page: %v", err)
	}
	defer page.Close()

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
				// TODO: Send notification using `ntfy`
				log.Info("New item", "title", item.Title, "guid", item.Guid, "link", item.Link)
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
		log.Error("Failed to fetch RSS feed", "error", err)
	}
	log.Info("Processing RSS feed...")
	err = ProcessRSSFeed(result)
	if err != nil {
		log.Error("Failed to process RSS feed", "error", err)
	}
	log.Info("Done!")
}
