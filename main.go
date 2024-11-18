package main

import (
	"encoding/xml"

	"github.com/charmbracelet/log"
	"github.com/playwright-community/playwright-go"
)

var page playwright.Page

type LETOffersRSSResult struct {
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

	// Get the text of the RSS feed
	resp, err := page.Goto("https://lowendtalk.com/categories/offers.rss")
	if err != nil {
		log.Fatalf("Failed to fetch text: %v", err)
	}
	body, err := resp.Text()
	if err != nil {
		log.Fatalf("Failed to get text: %v", err)
	}

	// Unmarshal the XML
	var result LETOffersRSSResult
	err = xml.Unmarshal([]byte(body), &result)
	if err != nil {
		log.Fatalf("Failed to unmarshal XML: %v", err)
	}

	// Log the results
	for _, item := range result.Channel.Items {
		log.Print(item.Title, "guid", item.Guid, "link", item.Link)
	}
}
