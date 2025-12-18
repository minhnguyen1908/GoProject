package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/joho/godotenv"
	"google.golang.org/api/customsearch/v1"
	"google.golang.org/api/option"

	"test-api/internal/logger"
)

// The API we will talk to
// Default to localhost, but can be overridden by env vars
var configMutex sync.RWMutex

// Config variables
var (
	APIQueueURL  = "http://localhost:8080"
	GoogleAPIKey = ""
	GoogleCXID   = ""
	// Feature flag for switching modes
	UseDynamicSearch = true
	// The hardcoded fallback query
	ManualQuery         = "Top 10 pet friendly restaurants in Ho Chi Minh City"
	MaxResultsToProcess = 1
)

// [NEW MODELS] ----
type SeederRequest struct {
	ID    string `json:"id"`
	Query string `json:"query"`
}

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

	// <--- CHANGE: Load Config from .env
	loadConfig()

	if getGoogleAPIKey() == "" || getGoogleCXID() == "" {
		log.Fatal("❌ Error: Missing Google key or cx")
	}

	log.Printf("🔗 Using API Base URL: %s", getAPIQueueURL())

	// [NEW LOGIC] ========
	// We start a server to LISTEN for commands
	http.HandleFunc("/process", processHandler)

	port := ":8081"
	log.Printf("👷 [Seeder] Worker ready. Listening on %s", port)
	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatal(err)
	}

	// ====================
	// [OLD CODE]
	// This was the old "Infinite Loop" that pulled data.
	/*
		reloadSig := make(chan os.Signal, 1)
		signal.Notify(reloadSig, syscall.SIGUSR1)

		go func() {
			for {
				<-reloadSig
				log.Println("🔄 [Config] Signal received! Reloading configuration...")
				loadConfig()
				log.Printf("✨ [Config] Reload complete. Limit: %d", getMaxResults())
			}
		}()

		for {
			runSeederCycle()
			log.Println("⏳ Waiting 10s before next cycle... (Send SIGUSR1 to reload config)")
			time.Sleep(10 * time.Second)
		}
	*/
	// =====================
}

// [NEW HANDLER] triggerd by API.
func processHandler(w http.ResponseWriter, r *http.Request) {
	var req SeederRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", 400)
		return
	}

	log.Printf("📥 [Seeder] Received Job: %s", req.Query)

	// Reply OK immediately
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Job Accepted"))

	go func() {
		performSearchAndReport(req)
	}()
}

func performSearchAndReport(req SeederRequest) {
	log.Printf("🔎 [Seeder] Googling: %s...", req.Query)

	// 1. Run Search
	// We pass 'true' for debug to see the raw response log!
	links, totalResults, err := googleSearch(req.Query, true)

	status := "done"
	if err != nil {
		log.Printf("❌ [Seeder] Search Failed: %v", err)
		status = "failed"
	} else {
		log.Printf("✅ [Seeder] Found %d links. Processing...", len(links))

		// [RESTORED LOGIC]: Sending links to queue is kept active!
		count := 0
		for _, link := range links {
			if count >= MaxResultsToProcess {
				break
			}
			if isSocialMedia(link) {
				continue
			}

			if err := sendToQueue(link); err != nil {
				log.Printf("⚠️ [Queue] Failed to send %s", link)
			} else {
				log.Printf("📤 [Queue] Sent: %s", link)
				count++
				time.Sleep(500 * time.Millisecond)
			}
		}
	}

	updateAPI(req.ID, status, totalResults, len(links))
}

// [OLD CODE] ===============
// This function is no longer called
/*
func runSeederCycle() {
	var currentQuery string
	var taskID string

	if getUseDynamicSearch() {
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
		currentQuery = getManualQuery()
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
	limit := getMaxResults()

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


	if getUseDynamicSearch() {
		if err := updateSearchTask(taskID, "done", totalResults, count); err != nil {
			log.Printf("❌ Failed to update task status: %v", err)
		} else {
			log.Println("🏁 Task completed and updated.")
		}
	} else {
		log.Println("🏁 Manual run completed.")
	}
}

func claimSearchTask(query string) (*SearchTask, error) {
	reqData := map[string]string{"query": query}
	jsonData, _ := json.Marshal(reqData)
	url := getAPIQueueURL() + "/search/claim"
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil { return nil, err }
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
*/

// ... (Getters) ...
func getGoogleAPIKey() string {
	configMutex.RLock()
	defer configMutex.RUnlock()
	return GoogleAPIKey
}
func getGoogleCXID() string {
	configMutex.RLock()
	defer configMutex.RUnlock()
	return GoogleCXID
}
func getAPIQueueURL() string {
	configMutex.RLock()
	defer configMutex.RUnlock()
	return APIQueueURL
}
func getUseDynamicSearch() bool {
	configMutex.RLock()
	defer configMutex.RUnlock()
	return UseDynamicSearch
}
func getManualQuery() string {
	configMutex.RLock()
	defer configMutex.RUnlock()
	return ManualQuery
}
func getMaxResults() int {
	configMutex.RLock()
	defer configMutex.RUnlock()
	return MaxResultsToProcess
}

// googleSearch uses the Official Google Go client to find links
func googleSearch(query string, debug bool) ([]string, string, error) {
	ctx := context.Background()
	svc, err := customsearch.NewService(ctx, option.WithAPIKey(GoogleAPIKey))
	if err != nil {
		return nil, "", fmt.Errorf("service create failed: %w", err)
	}

	// Perform the search
	resp, err := svc.Cse.List().Cx(getGoogleCXID()).Q(query).Do()
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

func updateAPI(id, status, estimated string, found int) error {
	reqData := SearchUpdate{
		ID:             id,
		Status:         status,
		TotalEstimated: estimated,
		ActualFound:    found,
	}
	jsonData, _ := json.Marshal(reqData)
	url := getAPIQueueURL() + "/search"
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

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

// <--- CHANGE: Config Loader using godotenv
func loadConfig() {
	// Load .env
	if err := godotenv.Load(); err != nil {
		log.Println("⚠️ No .env file found (using system env vars)")
	} else {
		log.Println("📂 Loaded config from .env")
	}

	newAPIKey := os.Getenv("G_SEARCH")
	newCXID := os.Getenv("G_CX")
	newAPIURL := os.Getenv("API_QUEUE_URL")

	newManualQuery := os.Getenv("MANUAL_QUERY")

	configMutex.Lock()
	defer configMutex.Unlock()

	if newAPIKey != "" {
		GoogleAPIKey = newAPIKey
	}

	if newCXID != "" {
		GoogleCXID = newCXID
	}
	if newAPIURL != "" {
		APIQueueURL = strings.TrimSuffix(newAPIURL, "/")
	}
	if newManualQuery != "" {
		ManualQuery = newManualQuery
	}

	// Environment override for flag
	if val := os.Getenv("USE_DYNAMIC_SEARCH"); val != "" {
		if boolVal, err := strconv.ParseBool(val); err == nil {
			UseDynamicSearch = boolVal
		}
	}
	if val := os.Getenv("MAX_RESULTS"); val != "" {
		if intVal, err := strconv.Atoi(val); err == nil {
			MaxResultsToProcess = intVal
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
	targetURL := getAPIQueueURL() + "/queue"
	resp, err := http.Post(targetURL, "application/json", bytes.NewBuffer(jsonData))
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
