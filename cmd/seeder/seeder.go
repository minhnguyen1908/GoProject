package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"

	"google.golang.org/api/customsearch/v1"
	"google.golang.org/api/option"

	"test-api/internal/logger"
)

// The API we will talk to
// Default to localhost, but can be overridden by env vars
var APIQueueURL = "http://localhost:8080/queue"

// Config variables
var (
	GoogleAPIKey = ""
	GoogleCXID   = ""
)

// The Payload structure (Must match what the API expects)
type EnqueueRequest struct {
	URL string `json:"url"`
}

func main() {
	// 1. Setup Logger
	logger.Setup("seeder.log")

	// 2. Load Credentials
	loadCredentials()

	if GoogleAPIKey == "" || GoogleCXID == "" {
		log.Fatal("❌ Error: Missing Google API key and CX")
	}

	log.Printf("🔗 Using API Queue URL: %s", APIQueueURL)

	// 3. Define the Search Query
	query := "top 10 pet friendly restaurants in Ho Chi Minh City"
	log.Printf("🔍 [Search] Querying Google for: '%s'", query)

	// 4. Perform Search
	links, err := googleSearch(query)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("✅ [Search] Found %d links via Google.", len(links))

	// 5. Filter & Send to Queue API
	for _, link := range links {
		if isSocialMedia(link) {
			log.Printf("🚫 [Filter] Skipping Social Media: %s", link)
			continue
		}

		log.Printf("📤 [Queue] Sending: %s", link)
		if err := sendToQueue(link); err != nil {
			log.Printf("⚠️ [Queue] Failed to send %s: %v", link, err)
		}
	}
}

// googleSearch uses the Official Google Go client to find links
func googleSearch(query string) ([]string, error) {
	ctx := context.Background()
	svc, err := customsearch.NewService(ctx, option.WithAPIKey(GoogleAPIKey))
	if err != nil {
		log.Printf("❌ Failed to create search service: %v", err)
		return nil, err
	}

	// Perform the search
	resp, err := svc.Cse.List().Cx(GoogleCXID).Q(query).Do()
	if err != nil {
		log.Printf("❌ Search failed: %v", err)
		return nil, err
	}

	var results []string
	for _, item := range resp.Items {
		results = append(results, item.Link)
	}
	return results, nil
}

// Helper to read secrets.nu file
func loadCredentials() {
	file, err := os.Open("secrets.nu")
	if err == nil {
		defer file.Close()
		log.Println("📂 Loading credentials from secrets.nu")

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := scanner.Text()

			// 1. Split by "="

			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				keyPart := strings.TrimSpace(parts[0])
				valuePart := strings.TrimSpace(parts[1])

				// 2. Clean up the Value (Remove quotes " or ')
				valuePart = strings.Trim(valuePart, `"'`)

				if strings.Contains(keyPart, "G_SEARCH") {
					GoogleAPIKey = valuePart
				}
				if strings.Contains(keyPart, "G_CX") {
					GoogleCXID = valuePart
				}
				if strings.Contains(keyPart, "API_QUEUE_URL") {
					APIQueueURL = valuePart
				}
			}
		}
	} else {
		log.Println("⚠️ Could not open secrets.nu, checking environment variables...")
	}

	// Environment variables override file
	if key := os.Getenv("G_SEARCH"); key != "" {
		GoogleAPIKey = key
	}
	if cx := os.Getenv("G_CX"); cx != "" {
		GoogleCXID = cx
	}
	if url := os.Getenv("API_QUEUE_URL"); url != "" {
		APIQueueURL = url
	}
}

// isSocialMedia checks if the URL belongs to a blocked domain
func isSocialMedia(url string) bool {
	blocked := []string{
		"facebook.com",
		"instagram.com",
		"twitter.com",
		"tiktok.com",
		"youtube.com",
		"tripadvisor.com",
	}
	for _, b := range blocked {
		if strings.Contains(url, b) {
			return true
		}
	}
	return false
}

// sendToQueue makes a POST request to your API
func sendToQueue(url string) error {
	payload := EnqueueRequest{URL: url}
	jsonData, _ := json.Marshal(payload)

	resp, err := http.Post(APIQueueURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("❌ API Request Failed: %v", err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		// Log detailed error to file
		log.Printf("❌ API Error Status: %d for URL %s", resp.StatusCode, url)
		// We return nil here to indicate "request finished", even if status was bad
		return nil
	}
	return nil
}
