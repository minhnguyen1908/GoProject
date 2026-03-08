## **Product Requirement Document: Niche Search & Extraction Engine**

### **1. The Problem Statement**

Users seeking highly specific, niche data (e.g., "Pet-friendly restaurants in HCMC") are often forced to manually browse multiple search results, deal with outdated blogs, and verify addresses one by one. Current AI solutions are either too expensive (high token costs from raw HTML) or hit API rate limits quickly on free tiers.

### **2. The Solution**

A Golang-based automated pipeline that accepts natural language queries, manages search volume via a transparent queuing system, and utilizes optimized LLM extraction to provide structured, verified results (PostgreSQL) while maintaining low operational costs (Token Truncation).

---

### **3. Core User Story (The PO Feature)**

> **As a** Niche Information Seeker,
> **I want to** submit a specific search query and see my position in the processing queue,
> **So that I can** receive a structured list of verified locations without having to manually filter through raw search results.

#### **Acceptance Criteria (The Architect's Logic):**

* **Query Input:** System must accept a string input (e.g., "Top 10 pet-friendly cafes in District 1").
* **Queue Visibility:** The UI/API must return the current "Queue Position" and "Total Pending Requests" to the user immediately.
* **Automated Processing:** The system must trigger the **Google Search API** and pull the top $N$ relevant URLs into **MongoDB**.
* **Resource Optimization:** Before LLM processing, the system must "trunk" (truncate) raw HTML to remove `head`, `script`, and `style` tags to reduce token consumption by at least **60-70%**.
* **Structured Extraction:** The final output must be extracted into a **PostgreSQL** schema containing: `Name`, `Address`, `Verified Status`, and `Last Updated`.

---

### **4. Technical Architecture Specifications (For the Portfolio)**

| Component | Technology | Purpose |
| --- | --- | --- |
| **Language** | **Golang** | High-concurrency processing for crawling and API management. |
| **Containerization** | **nerdctl + Buildkit** | Modern, daemon-less container management for the microservices. |
| **Queue Database** | **MongoDB** | Fast, schema-less storage for tracking current request states. |
| **Truth Database** | **PostgreSQL** | Relational storage for the final, structured, and verified data. |
| **Cost Logic** | **HTML Truncation** | Custom logic to strip non-content tags before passing data to the LLM SDK. |

---

### **5. Product Constraints & "Free Tier" Strategy**

* **Rate Limiting:** Due to Google Search API free-tier limits, the system will process requests in a serial queue.
* **Transparency:** The system will "Update Status" in the DB at every stage (Pending -> Searching -> Truncating -> Extracting -> Finished).

---

### **6. Cost Management & API Governance**

#### **The "100-Limit" Strategy**
To maintain a zero-cost operational model, the system leverages the Google Search API Free Tier. 
* **Constraint:** 100 queries per 24-hour period.
* **Logic:** The API service implements a `QuotaUsage` check in MongoDB before dispatching any job to the Seeder. If the limit is reached, jobs remain in `pending` status until the quota resets.

#### **User Stories**
* **Financial Safety:** As a Product Owner, I want the system to automatically halt searches when the 100-query limit is reached, so that I do not incur unexpected API charges.
* **Transparency:** As a User, I want to be notified if my search is delayed due to daily quota limits, so I understand why my request hasn't been processed yet.
* **Efficiency:** As a Developer, I want to log every successful search against the daily quota, so I can audit usage patterns and plan for future scaling (e.g., adding multiple API keys).

---

### **7. Definition of Done (DoD)**

A feature or task is considered **"Done"** only when it meets the following criteria across development and product standards:

#### **A. Code & Logic (Technical Standard)**
- [ ] **Functional Requirement:** The Go code performs the task as described in the User Story (e.g., the quota check correctly blocks requests after 100).
- [ ] **Database Integrity:** All state changes (switching job status) are successfully committed to MongoDB.
- [ ] **Quota Accuracy:** For every Google Search performed, the `QuotaUsage` counter is incremented accurately for the current date.

#### **B. Quality & Observability (Reliability Standard)**
- [ ] **Error Handling:** The code includes guard clauses to handle empty queries, missing API keys, or database connection timeouts.
- [ ] **Structured Logging:** Every major action (API calls, job dispatches) is recorded via the `internal/logger` and visible in `api.log`.
- [ ] **Status Transparency:** A user can verify the result through the API endpoints and see the updated job status or found URLs.

#### **C. Documentation (Portfolio Standard)**
- [ ] **PRD Updated:** Any new feature logic is added to the "Key Features" or "Roadmap" section of this document.
- [ ] **README Alignment:** The root `README.md` is updated if there is a change to the tech stack or core project value.

---

### **8. Product Backlog & Task Tracking**

This section tracks pending features and technical improvements. Tasks are moved to "Completed" once they meet the Definition of Done (Section 7).

#### **🔥 High Priority (Sprint 1)**
- [ ] **Dynamic Query Injection:** Update `seeder.go` to process the actual `Query` field from the API instead of the hardcoded "pet friendly" string.
- [ ] **Daily Quota Reset Logic:** Implement a robust check to ensure the `QuotaUsage` count resets to 0 when the date changes.
- [ ] **Basic Error Mapping:** Ensure the API returns a clear message when the Google Search quota is exhausted.

#### **⚡ Medium Priority (Sprint 2)**
- [ ] **Multi-Result Storage:** Allow the system to process and save more than 1 result per search (currently hardcoded to 1).
- [ ] **Search Metadata Persistence:** Save snippets and page titles from Google results into MongoDB, not just the URL.

#### **❄️ Future Ideas (Icebox)**
- [ ] **Notification System:** Integrate Slack or Email alerts for completed high-priority jobs.
- [ ] **User Dashboard:** Create a simple interface to manage search tasks without using terminal commands.

#### **✅ Completed Tasks (Proven Features)**

These features are already fully implemented in the Go codebase and meet our current technical standards:

- [x] **🏗️ Distributed Architecture:** Successfully decoupled the system into an **API Service** (Management) and a **Seeder Service** (Worker) using a microservices-style pattern.
- [x] **🔍 Google Search Integration:** Implemented live data fetching using the official Google Custom Search JSON API.
- [x] **🛡️ Quota Governance:** Developed the core logic to enforce the **100-request daily limit**, protecting the project from unexpected API costs.
- [x] **🗄️ Database Foundation:** Established a robust MongoDB connection to persist search tasks and track real-time quota usage.
- [x] **🔄 Job Lifecycle Management:** Built a state machine to track tasks through various stages: `pending` ➡️ `processing` ➡️ `done` or `failed`.
- [x] **📑 Structured Logging:** Integrated a professional logging system (Zap + Lumberjack) to provide audit trails and automatic file rotation for `api.log`.
- [x] **📡 RESTful API Endpoints:** Developed GIN-based routes allowing users to query job statuses and update data programmatically.
- [x] **🐳 Containerization:** Fully dockerized the environment with `compose.yml` for seamless deployment across different machines.
