package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

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
	// Feature flag for switching modes
	UseDynamicSearch = true
	// The hardcoded fallback query
	ManualQuery = "Top 10 pet friendly restaurants in Ho Chi Minh City"
)

// Limit for testing (Safety Brake)
// Set to 10 for full run, or 1 for testing.
const MaxResultsToProcess = 1

// The Payload structure (Must match what the API expects)
type EnqueueRequest struct {
	URL string `json:"url"`
}

// Add Search Task Models to talk to API
type SearchTask struct {
	ID    string `json:"id"`
	Query string `json:"query"`
}

type SearchTaskResponse struct {
	Task *SearchTask `json:"task"`
}

type SearchUpdate struct {
	ID             string `json:"id"`
	Status         string `json:"status"`
	TotalEstimated string `json:"total_estimated"`
	ActualFound    int    `json:"actual_found"`
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
	var currentQuery string
	var taskID string

	if UseDynamicSearch {
		log.Println("🔄 Mode: Dynamic (Asking API for work)")

		// Claim a Search Task from the API
		task, err := claimSearchTask("")
		if err != nil {
			log.Printf("⚠️ No pending search tasks found: %v", err)
			log.Println("💡 Tip: Add a task via POST /search first, or I will stop here.")
			return
		}

		currentQuery = task.Query
		taskID = task.ID
		log.Printf("🚀 [Seeder] Claimed Task: '%s' (ID: %s)", currentQuery, taskID)
	} else {
		log.Println("🔧 Mode: Manual (Using hardcoded query)")
		currentQuery = ManualQuery
		taskID = "manual-run"
		log.Printf("🚀 [Seeder] Starting Manual Run: '%s'", currentQuery)
	}

	// 4. Perform Search
	links, totalResults, err := googleSearch(currentQuery, true)
	if err != nil {
		log.Printf("❌ Search failed: %v", err)
		// Report failure back to API? For now just stop.
		return
	}

	log.Printf("✅ [Search] Found %d links. Google Est: %s", len(links), totalResults)

	// 5. Filter & Send to Queue API
	count := 0
	for _, link := range links {
		// Safety Brake logic
		if count >= MaxResultsToProcess {
			log.Printf("🛑 [Limit] Reached testing limit of %d. Stopping.", MaxResultsToProcess)
			break
		}
		if isSocialMedia(link) {
			log.Printf("🚫 [Filter] Skipping Social Media: %s", link)
			continue
		}

		log.Printf("📤 [Queue] Sending: %s", link)
		if err := sendToQueue(link); err != nil {
			log.Printf("⚠️ [Queue] Failed to send %s: %v", link, err)
		} else {
			count++
			// Polite deplay between API calls during testing
			time.Sleep(1 * time.Second)
		}
	}

	// Update the Task status (Only if Dynamic)
	if UseDynamicSearch {
		if err := updateSearchTask(taskID, "done", totalResults, count); err != nil {
			log.Printf("❌ Failed to update task status: %v", err)
		} else {
			log.Println("🏁 Task completed and updated.")
		}
	} else {
		log.Println("🏁 Manual run completed.")
	}
}

// googleSearch uses the Official Google Go client to find links
func googleSearch(query string, debug bool) ([]string, string, error) {
	ctx := context.Background()
	svc, err := customsearch.NewService(ctx, option.WithAPIKey(GoogleAPIKey))
	if err != nil {
		return nil, "", fmt.Errorf("service create failed: %w", err)
	}

	// Perform the search
	resp, err := svc.Cse.List().Cx(GoogleCXID).Q(query).Do()
	if err != nil {
		return nil, "", fmt.Errorf("search call failed: %w", err)
	}

	// Debug Logging
	// if debug is true, we dump the entire Google response to the log
	if debug {
		// Marshal the struct back to JSON to see what Google sent us
		log.Printf("🐛 [Debug]Search Info: TotalResults=%s, SearchTime=%f",
			resp.SearchInformation.TotalResults, resp.SearchInformation.SearchTime)
	}

	var results []string
	for _, item := range resp.Items {
		results = append(results, item.Link)
	}
	return results, resp.SearchInformation.TotalResults, nil
}

func claimSearchTask(query string) (*SearchTask, error) {
	reqData := map[string]string{"query": query}
	jsonData, _ := json.Marshal(reqData)

	resp, err := http.Post(APIQueueURL+"/search/claim", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("api status %d", resp.StatusCode)
	}

	var apiResp SearchTaskResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, err
	}

	if apiResp.Task == nil {
		return nil, fmt.Errorf("empty task received")
	}

	return apiResp.Task, nil
}

func updateSearchTask(id, status, estimated string, found int) error {
	reqData := SearchUpdate{
		ID:             id,
		Status:         status,
		TotalEstimated: estimated,
		ActualFound:    found,
	}
	jsonData, _ := json.Marshal(reqData)

	req, err := http.NewRequest(http.MethodPut, APIQueueURL+"/search", bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	req.Header.Set("Context-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("api status %d", resp.StatusCode)
	}
	return nil
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
					APIQueueURL = strings.TrimSuffix(valuePart, "/")
				}
				// Parse the boolean flag from file
				if strings.Contains(keyPart, "USE_DYNAMIC_SEARCH") {
					if val, err := strconv.ParseBool(valuePart); err == nil {
						UseDynamicSearch = val
					}
				}
				// Allow overriding manual query from file
				if strings.Contains(keyPart, "MANUAL_QUERY") {
					ManualQuery = valuePart
				}
			}
		}
	}

	// Environment variables override file
	if key := os.Getenv("G_SEARCH"); key != "" {
		GoogleAPIKey = key
	}
	if cx := os.Getenv("G_CX"); cx != "" {
		GoogleCXID = cx
	}
	if url := os.Getenv("API_QUEUE_URL"); url != "" {
		APIQueueURL = strings.TrimSuffix(url, "/")
	}

	// Environment override for flag
	if val := os.Getenv("USE_DYNAMIC_SEARCH"); val != "" {
		if boolVal, err := strconv.ParseBool(val); err == nil {
			UseDynamicSearch = boolVal
		}
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
