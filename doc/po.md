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
