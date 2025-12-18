package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	// Import our shared tool (Using project name, not folder name)
	"test-api/internal/logger"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	MongoURI  = "mongodb://localhost:27017"
	SeederURL = "http://localhost:8081"
)

const (
	// --- Job Statuses ---
	StatusPending    = "pending"
	StatusProcessing = "processing"
	StatusDone       = "done"
	StatusFailed     = "failed"

	// Add Quota Constant
	GoogleDailyLimit = 100
)

type CrawlJob struct {
	URL       string    `json:"url" bson:"url"`
	Status    string    `json:"status" bson:"status"`
	CreatedAt time.Time `json:"created_at" bson:"created_at"`
}

// Represents a keyword search we want the Seeder to perform.
// We track estimated results vs actual results here.
type SearchTask struct {
	ID             primitive.ObjectID `json:"id" bson:"_id,omitempty"`
	Query          string             `json:"query" bson:"query"`
	Status         string             `json:"status" bson:"status"`
	TotalEstimated string             `json:"total_estimated" bson:"total_estimated"`
	ActualFound    int                `json:"actual_found" bson:"actual_found"`
	CreatedAt      time.Time          `json:"created_at" bson:"created_at"`
	UpdatedAt      time.Time          `json:"updated_at" bson:"updated_at"`
}

// QuotaUsage Model
type QuotaUsage struct {
	ID        string    `bson:"_id"`  // e.g. "google search"
	Date      string    `bson:"date"` // e.g. "2025-12-17"
	Count     int       `bson:"count"`
	UpdatedAt time.Time `bson:"updated_at"`
}

// HATEOAS structure
// Link represents a possible action the client can take.
type Link struct {
	Rel    string `json:"rel"`    // Relationship (e.g., "self", "claim", "update")
	Method string `json:"method"` // HTTP Method (GET, POST, PUT)
	HRef   string `json:"href"`   // The URL endpoint
}

// JobResponse wraps the data with HATEOAS links.
type JobResponse struct {
	CrawlJob        // Embed the orginal data (Inheritance-ish)
	Links    []Link `json:"_links"`
}

// Global variables
var queueCol *mongo.Collection

// collection for search tasks
var searchCol *mongo.Collection

// collection for QuotaUsage
var quotaCol *mongo.Collection
var mongoClient *mongo.Client

// ==========================================
// Main function
// ==========================================

func main() {
	// 1. Start the Logger FIRST
	// This creates 'api.log' and connects it to the terminal.
	logger.Setup("api.log")

	// Load Config from Environment (Security Best Practice)
	loadConfig()

	// 2. Connect DB
	initMongo()
	r := gin.Default()
	// Handle 404 for undefined routes
	r.NoRoute(notFoundHandler)

	// --- ROUTES ---
	r.GET("/health", healthHandler)
	r.POST("/queue", enqueueHandler)
	r.GET("/queue", searchJobsHandler)
	r.POST("/queue/claim", claimJobHandler)
	r.PUT("/queue", updateJobHandler)

	// Added Routes for Seeder Tasks
	r.POST("/search", createSearchHandler)
	r.POST("/search/claim", claimSearchHandler)
	r.PUT("/search", updateSearchHandler)

	// Quota Routes
	r.GET("/quota/google", getQuotaHandler)
	r.POST("/quota/google/increment", incrementQuotaHandler)

	// SERVER SETUP (HTTPS + GRACEFUL SHUTDOWN)

	// Check for HTTPS Certificates.
	certFile := "cert.pem"
	keyFile := "key.pem"
	useTLS := fileExists(certFile) && fileExists(keyFile)

	// Graceful Shutdown Setup
	//Create the HTTP server manually so we can control it.
	srv := &http.Server{
		Addr:    ":8080",
		Handler: r,
	}

	if useTLS {
		srv.Addr = ":8443" // Standard HTTPS alt port
		log.Println("🔒 Certificates found! Starting in HTTPS mode on: 8443")

		// Run the server in a separate Goroutine so it doesn't block
		go func() {
			if err := srv.ListenAndServeTLS(certFile, keyFile); err != nil && err != http.ErrServerClosed {
				log.Fatalf("❌ HTTPS Server Error: %s\n", err)
			}
		}()
	} else {
		srv.Addr = ":8080"
		log.Println("🔓 No certs found. Starting in HTTP mode on: 8080")
		go func() {
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatalf("❌ HTTP Server Error: %s\n", err)
			}
		}()
	}

	// Wait for interrupt signal to gracefully shutdown the server with a timeout of 5 seconds.
	quit := make(chan os.Signal, 1)

	// signal.Notify tell Go: "if you see SIGINT (Ctrl + C) or SIGTERM (Docker Stop), sent it to 'quit' channel".
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// athis line BLOCKS until a signal is received.
	<-quit
	log.Println("🛑 Shutting down server...")

	// The context is use to inform the server it has 5 seconds to finish the request it is currently handling.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("❌ Server Forced to Shutdown: ", err)
	}

	// Disconnect Mongo safely
	if err := mongoClient.Disconnect(ctx); err != nil {
		log.Printf("⚠️ Mongo Disconnect Error: %v", err)
	}

	log.Println("👋 Server exiting")
}

// ==========================================
// HANDLERS (The Logic)
// ==========================================

// healthHandler checks if the server is alive.
func healthHandler(c *gin.Context) {
	// Log the check
	log.Println("🩺 [Health] Checking system status...")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := mongoClient.Ping(ctx, nil); err != nil {
		log.Printf("❌ [Health] FAILED: Mongo unreachable. Error: %v", err)
		sendResponse(c, http.StatusServiceUnavailable, gin.H{
			"status": "unhealthy",
			"error":  "Database unreachable",
		})
		return
	}

	sendResponse(c, http.StatusOK, gin.H{
		"status": "healthy",
		"uptime": time.Since(startTime).String(),
		// Even the health check can have links!
		"_links": []Link{
			{Rel: "queue", Method: "GET", HRef: "/queue"},
			{Rel: "enqueue", Method: "POST", HRef: "/queue"},
		},
	})
}

func enqueueHandler(c *gin.Context) {
	var job CrawlJob
	// bindJSON handles the error logging for bad input
	if !bindJSON(c, &job) {
		return
	}

	// Log the Intent
	log.Printf("📥 [Enqueue] Request received: URL=%s", job.URL)

	job.Status = StatusPending
	job.CreatedAt = time.Now()

	_, err := queueCol.InsertOne(c.Request.Context(), job)

	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			log.Printf("⚠️ [Enqueue] Duplicate skipped: %s", job.URL)
			sendResponse(c, http.StatusConflict, gin.H{"message": "URL already in queue (Skipped)"})
			return
		}
		if handleDBError(c, err, "Mongo Insert Error") {
			return
		}
	}

	log.Printf("✅ [Enqueue] Success: %s", job.URL)
	// Return HATEOAS response
	response := NewJobResponse(job)
	sendResponse(c, http.StatusCreated, gin.H{
		"message": "Job queued successfully!",
		"job":     response,
	})
}

// GET /queue - Search for jobs.
func searchJobsHandler(c *gin.Context) {
	// Log the Search params
	queryParams := c.Request.URL.Query()
	log.Printf("🔍 [Search] Query: %v", queryParams)

	var jobs []CrawlJob

	allowedKeys := map[string]bool{"status": true, "url": true}

	filter := bson.M{}

	for key, values := range queryParams {
		if len(values) == 0 {
			continue
		}

		if !allowedKeys[key] {
			errorMsg := "Invalid filter: " + key
			sendResponse(c, http.StatusBadRequest, gin.H{"error": errorMsg})
			return
		}

		if key == "url" {
			filter[key] = primitive.Regex{Pattern: values[0], Options: "i"}
		} else {
			if len(values) > 1 {
				filter[key] = bson.M{"$in": values}
			} else {
				filter[key] = values[0]
			}
		}
	}

	cursor, err := queueCol.Find(c.Request.Context(), filter)
	if handleDBError(c, err, "Mongo Search Error") {
		return
	}

	defer cursor.Close(c.Request.Context())

	err = cursor.All(c.Request.Context(), &jobs)
	if handleDBError(c, err, "Cursor Decode Error") {
		return
	}

	log.Printf("✅ [Search] Found %d jobs matching filter ", len(jobs))

	var jobResources []JobResponse
	for _, job := range jobs {
		jobResources = append(jobResources, NewJobResponse(job))
	}

	if jobResources == nil {
		jobResources = []JobResponse{}
	}

	sendResponse(c, http.StatusOK, gin.H{
		"count": len(jobs),
		"jobs":  jobResources,
		"_links": []Link{
			{Rel: "self", Method: "GET", HRef: "/queue"},
			{Rel: "claim", Method: "POST", HRef: "/queue/claim"},
		},
	})

}

// POST /queue/claim - Claim a job for processing.
func claimJobHandler(c *gin.Context) {
	var job CrawlJob

	var claimReq struct {
		URL string `json:"url"`
	}

	// Log what the worker is asking for
	log.Printf("👷‍♀️ [Claim] Worker request. Specific URL: '%s'", claimReq.URL)

	if !bindJSON(c, &claimReq) {
		return
	}

	filter := bson.M{"status": StatusPending}

	// Exact Match (Safer)
	if claimReq.URL != "" {
		filter["url"] = claimReq.URL
	}

	update := bson.M{"$set": bson.M{"status": StatusProcessing}}

	// add sorting to grab the oldest one.
	opts := options.FindOneAndUpdate().
		SetReturnDocument(options.After).
		SetSort(bson.M{"created_at": 1}) // `1` for ascending and `-1` for descending order.

	err := queueCol.FindOneAndUpdate(c.Request.Context(), filter, update, opts).Decode(&job)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			log.Printf("⚠️ [Claim] No jobs available (Filter: %v)", filter)
			msg := "No pending jobs found in queue."
			if claimReq.URL != "" {
				msg = "No pending job found matching URL: '" + claimReq.URL + "'"
			}
			sendResponse(c, http.StatusNotFound, gin.H{"status": "failed", "message": msg})
			return
		}
		if handleDBError(c, err, "Mongo Claim Error") {
			return
		}
	}

	log.Printf("✅ [Claim] Assigned job: %s", job.URL)
	sendResponse(c, http.StatusOK, NewJobResponse(job))
}

// PUT method
func updateJobHandler(c *gin.Context) {
	var updateData struct {
		URL    string `json:"url"`
		Status string `json:"status"`
	}

	if !bindJSON(c, &updateData) {
		return
	}

	log.Printf("🔄 [Update] Job: %s -> New Status: %s", updateData.URL, updateData.Status)

	allowUpdates := map[string]bool{StatusDone: true, StatusFailed: true}
	if !allowUpdates[updateData.Status] {
		sendResponse(c, http.StatusBadRequest, gin.H{
			"error":   "Invalid status transition",
			"allowed": []string{StatusDone, StatusFailed},
		})
		return
	}

	filter := bson.M{"url": updateData.URL}
	update := bson.M{"$set": bson.M{"status": updateData.Status}}

	result, err := queueCol.UpdateOne(c.Request.Context(), filter, update)
	if handleDBError(c, err, "Mongo Update Error") {
		return
	}

	if result.MatchedCount == 0 {
		log.Printf("⚠️ [Update] Job not found: %s", updateData.URL)
		sendResponse(c, http.StatusNotFound, gin.H{"error": "Job not found"})
		return
	}

	log.Println("✅ [Update] Success.")
	sendResponse(c, http.StatusOK, gin.H{"message": "Job updated", "new_status": updateData.Status})
}

// Handler to Create a new Search Task
func createSearchHandler(c *gin.Context) {
	var task SearchTask
	if !bindJSON(c, &task) {
		return
	}

	task.ID = primitive.NewObjectID() // Generat ID manually so we can return it
	task.Status = StatusPending
	task.CreatedAt = time.Now()
	task.UpdatedAt = time.Now()

	_, err := searchCol.InsertOne(c.Request.Context(), task)
	if handleDBError(c, err, "Search Insert Error") {
		return
	}

	// [NEW LOGIC]
	// We call the dispatcher immediately
	go dispatchJob()

	log.Printf("✅ [Search] New Task Created: %s", task.Query)
	sendResponse(c, http.StatusCreated, gin.H{
		"message": "Search task created",
		"task_id": task.ID,
	})
}

// Handler for Seeder to Claim a Search Task.
func claimSearchHandler(c *gin.Context) {
	var task SearchTask

	// We allow claiming a specific query if needed, otherwise grab any pending.
	var req struct {
		Query string `json:"query"`
	}
	if !bindJSON(c, &req) {
		return
	}

	filter := bson.M{"status": StatusPending}
	if req.Query != "" {
		filter["query"] = req.Query
	}

	update := bson.M{
		"$set": bson.M{
			"status":     StatusProcessing,
			"updated_at": time.Now(),
		},
	}

	opts := options.FindOneAndUpdate().SetReturnDocument(options.After).SetSort(bson.M{"created_at": 1})

	err := searchCol.FindOneAndUpdate(c.Request.Context(), filter, update, opts).Decode(&task)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			sendResponse(c, http.StatusNotFound, gin.H{"message": "No pending search tasks"})
			return
		}
		if handleDBError(c, err, "Search Claim Error") {
			return
		}
	}

	log.Printf("✅ [Search] Task Claimed: %s", task.Query)
	sendResponse(c, http.StatusOK, gin.H{"task": task})
}

// Handler to Update Search Task
func updateSearchHandler(c *gin.Context) {
	var updateData struct {
		ID             string `json:"id" binding:"required"` // Must have ID to update
		Status         string `json:"status"`
		TotalEstimated string `json:"total_estimated"`
		ActualFound    int    `json:"actual_found"`
	}
	if !bindJSON(c, &updateData) {
		return
	}

	// Convert string ID to ObjectID
	objID, err := primitive.ObjectIDFromHex(updateData.ID)
	if err != nil {
		sendResponse(c, http.StatusBadRequest, gin.H{"error": "Invalid ID format"})
		return
	}

	filter := bson.M{"_id": objID}

	// Dynamic update: Only update fields that were sent
	updateFields := bson.M{"updated_at": time.Now()}
	if updateData.Status != "" {
		updateFields["status"] = updateData.Status
	}
	if updateData.TotalEstimated != "" {
		updateFields["total_estimated"] = updateData.TotalEstimated
	}
	if updateData.ActualFound > 0 {
		updateFields["actual_found"] = updateData.ActualFound
	}

	result, err := searchCol.UpdateOne(c.Request.Context(), filter, bson.M{"$set": updateFields})
	if handleDBError(c, err, "Search Update Error") {
		return
	}

	if result.MatchedCount == 0 {
		sendResponse(c, http.StatusNotFound, gin.H{"error": "Task not found"})
		return
	}

	// [NEW LOGIC]
	// We handle Quota counting and Triggering the next job here.
	if updateData.Status == StatusDone {
		log.Println("📈 [Quota] Search Done. Incrementing quota.")
		today := time.Now().Format("2025-12-17")

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_, err := quotaCol.UpdateOne(ctx,
			bson.M{"_id": "google_search"},
			bson.M{"$inc": bson.M{"count": 1}, "$set": bson.M{"date": today}},
			options.Update().SetUpsert(true))

		if err != nil {
			log.Printf("⚠️ [Quota] Failed to increment: %v", err)
		}
		go dispatchJob()
	}
	// ------------

	log.Printf("✅ [Search] Task Updated: %s", updateData.ID)
	sendResponse(c, http.StatusOK, gin.H{"message": "Search task updated"})
}

// Quota Handler

func getQuotaHandler(c *gin.Context) {
	today := time.Now().Format("2025-12-17")
	var usage QuotaUsage

	filter := bson.M{"_id": "google_search", "date": today}
	err := quotaCol.FindOne(c.Request.Context(), filter).Decode(&usage)

	currentCount := 0
	if err == nil {
		currentCount = usage.Count
	} else if err != mongo.ErrNoDocuments {
		handleDBError(c, err, "Quota Check Error")
		return
	}

	remaining := GoogleDailyLimit - currentCount
	if remaining < 0 {
		remaining = 0
	}

	sendResponse(c, http.StatusOK, gin.H{
		"service":    "google_search",
		"date":       today,
		"used":       currentCount,
		"limit":      GoogleDailyLimit,
		"remaining":  remaining,
		"can_search": remaining > 0,
	})
}

// Handler for Incrementing Quota
func incrementQuotaHandler(c *gin.Context) {
	today := time.Now().Format("2025-12-17")

	// Reset logic check
	var usage QuotaUsage
	filter := bson.M{"_id": "google_search"}

	err := quotaCol.FindOne(c.Request.Context(), filter).Decode(&usage)
	if err == nil {
		if usage.Date != today {
			// New Day detected
			usage.Count = 0
			usage.Date = today
		}
		if usage.Count >= GoogleDailyLimit {
			sendResponse(c, http.StatusTooManyRequests, gin.H{"error": "Daily quota exceeded", "limit": GoogleDailyLimit})
			return
		}
	} else if err != mongo.ErrNoDocuments {
		handleDBError(c, err, "Quota Read Error")
		return
	}

	update := bson.M{
		"$inc": bson.M{"count": 1},
		"$set": bson.M{"date": today, "updated_at": time.Now()},
	}
	opts := options.Update().SetUpsert(true)

	_, err = quotaCol.UpdateOne(c.Request.Context(), filter, update, opts)
	if handleDBError(c, err, "Quota Increment Error") {
		return
	}

	log.Printf("📈 [Quota] Google Search +1, Date: %s", today)
	sendResponse(c, http.StatusOK, gin.H{"message": "Quota incremented"})
}

// [NEW LOGIC] DISPATCHER LOGIC
func dispatchJob() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 1. Quota check
	var usage QuotaUsage
	err := quotaCol.FindOne(ctx, bson.M{"_id": "google_search"}).Decode(&usage)
	if err == nil && usage.Count >= GoogleDailyLimit {
		log.Printf("🛑 [Dispatch] Quota limit reach (%d).", usage.Count)
		return
	}

	// 2. Busy Check
	count, _ := searchCol.CountDocuments(ctx, bson.M{"status": StatusProcessing})
	if count > 0 {
		log.Println("⏸️ [Dispatch] Seeder is busy.")
		return
	}

	// 3. Find Next Job
	var task SearchTask
	opts := options.FindOneAndUpdate().SetSort(bson.M{"created_at": 1})
	filter := bson.M{"status": StatusPending}
	update := bson.M{"$set": bson.M{"status": StatusProcessing, "updated_at": time.Now()}}

	err = searchCol.FindOneAndUpdate(ctx, filter, update, opts).Decode(&task)
	if err != nil {
		return
	} // no pending jobs

	log.Printf("🚀 [Dispatch] Pushing task to Seeder: %s", task.Query)

	// 4. Push to Seeder
	payload := map[string]string{
		"id":    task.ID.Hex(),
		"query": task.Query,
	}
	jsonData, _ := json.Marshal(payload)

	target := SeederURL + "/process"
	resp, err := http.Post(target, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("❌ [Dispatch] Failed to contract Seeder: %v", err)
		return
	}
	defer resp.Body.Close()
}

// notFoundHandler - used to help user when they hit a non-existing route.
func notFoundHandler(c *gin.Context) {
	requestedPath := c.Request.URL.Path
	log.Println(">>> ⚠️ 404 - Route not found:", requestedPath)
	sendResponse(c, http.StatusNotFound, gin.H{
		"error":          "Route not found",
		"message":        "The URL you requested does not exist.",
		"requested_path": requestedPath,
	})
}

// DB setup

var startTime time.Time

func initMongo() {
	startTime = time.Now()

	log.Printf("⏳ [DB] Attempting to connect to MongoDB at: [%s]", MongoURI)

	var err error

	mongoClient, err = mongo.Connect(context.TODO(), options.Client().ApplyURI(MongoURI))
	if err != nil {
		log.Fatal("❌ [DB] Failed to create Mongo client: ", err)
	}

	ctx, cancel := context.WithTimeout(context.TODO(), 5*time.Second)
	defer cancel()

	if err := mongoClient.Ping(ctx, nil); err != nil {
		log.Printf("❌ [DB] Connection Failed!")
		log.Fatal(err) // Crash with the error details
	}

	db := mongoClient.Database("data_pool")
	queueCol = db.Collection("crawl_queue")
	searchCol = db.Collection("search_tasks")
	quotaCol = db.Collection("system_stats")

	// URL index
	urlIndex := mongo.IndexModel{
		Keys:    bson.M{"url": 1},
		Options: options.Index().SetUnique(true),
	}
	_, err = queueCol.Indexes().CreateOne(context.TODO(), urlIndex)
	if err != nil {
		log.Println("⚠️ [DB] Notice: URL index might already exist:", err)
	}

	// Status index
	statusIndex := mongo.IndexModel{
		Keys: bson.M{"status": 1},
	}
	_, err = queueCol.Indexes().CreateOne(context.TODO(), statusIndex)
	if err != nil {
		log.Println("⚠️ [DB] Notice: Status index might already exist:", err)
	}

	log.Println("✅ [DB] Connected to MongoDB successfully!")
}

// getMongoURI constructs the MongoDB connection string.
func getMongoURI() string {
	// Simple getter now, as Docker Compose/Env handles the complexity
	return MongoURI
}

// ==========================================
// HELPER FUNCTIONS
// ==========================================

// Get DB criteria from local variables.
func loadConfig() {
	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Println("⚠️ No .env file found (using system env vars)")
	} else {
		log.Println("📂 Loaded config from .env")
	}

	// Read variables into globals
	if uri := os.Getenv("MONGO_URI"); uri != "" {
		MongoURI = uri
	}
	if url := os.Getenv("SEEDER_URL"); url != "" {
		SeederURL = url
	}
}

// Search for HTTPS keys file "*.pem".
func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

// New Helper for HATEOAS Logic.
// athis function acts like a "Smart Menu Generator".
// It looks at the job's status and decides what links to show.
func NewJobResponse(job CrawlJob) JobResponse {
	res := JobResponse{
		CrawlJob: job,
		Links:    []Link{},
	}

	// Always add 'self'
	// Note: we use query params for GET because we don't have IDs in the path
	res.Links = append(res.Links, Link{
		Rel: "self", Method: "GET", HRef: "/queue?url=" + job.URL,
	})

	if job.Status == StatusPending {
		res.Links = append(res.Links, Link{
			Rel: "claim", Method: "POST", HRef: "/queue/claim",
		})
	} else if job.Status == StatusProcessing {
		res.Links = append(res.Links, Link{
			Rel: "update", Method: "PUT", HRef: "/queue",
		})
	}

	return res
}

// sendResponse is a wrapper around c.JSON.
// It makes our code cleaner and allows us to change response logic in one place later.
// c: The context (The Traffic Cop)
// status: The HTTP status code (e.g., 200, 400, 500)
// data: The content to send (can be a struct, map, or gin.H)
func sendResponse(c *gin.Context, status int, data interface{}) {
	c.JSON(status, data)
}

// bindJSON tries to bind the request body.
// Returns TRUE if successful, FALSE if failed (and sends response).
func bindJSON(c *gin.Context, obj interface{}) bool {
	// Standard Decoder from "encoding/json"
	decoder := json.NewDecoder(c.Request.Body)

	// The BOUNCER: If you send a field I don't know, Get Out!
	log.Println("👷‍♀️ I'm here, before the `DisallowUnknownFields`")
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(obj); err != nil {
		log.Printf("⚠️ [Bind] Bad Request: %v", err)
		sendResponse(c, http.StatusBadRequest, gin.H{"error": "Invalid JSON or Unknown Field" + err.Error()})
		return false
	}
	return true
}

// handleDBError checks if err is nil.
// If ERROR: logs it, seends 500 response, returns TRUE.
// If OK: Returns FALSE.
func handleDBError(c *gin.Context, err error, logPrefix string) bool {
	if err != nil {
		log.Printf("❌ [%s] Error: %v", logPrefix, err)
		sendResponse(c, http.StatusInternalServerError, gin.H{"error": "Database error"})
		return true
	}
	return false
}
