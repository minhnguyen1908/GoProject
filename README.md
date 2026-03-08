# 🚀 Gopher-Search: Automated Niche Lead Discovery

A distributed Go-based system designed to automate niche data extraction (e.g., "Pet-friendly restaurants") while strictly managing API costs.

## 🛠 Tech Stack
- **Language:** Golang (High-concurrency worker pattern)
- **Database:** MongoDB (Task queuing & Quota tracking)
- **Architecture:** Microservices (API + Seeder Worker)
- **Infrastructure:** Docker & Nginx

## 🌟 Key Features
- **Smart Quota Management:** Automatically enforces a 100-request daily limit to stay within the Google Search API free tier.
- **Distributed Worker Pattern:** Separates the management API from the search worker for better scalability.
- **Professional Logging:** Structured logging using `lumberjack` for file rotation and audit trails.

## 📂 Documentation
For a deep dive into the Product Owner strategy, user stories, and roadmap, see our [Product Requirement Document (PRD)](doc/po.md).
