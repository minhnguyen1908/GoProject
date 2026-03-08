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

## 👔 Product Management Highlights
This project follows a professional **Agile/Product Owner** framework:
- **Cost-Efficiency First:** Designed around the Google Search API free-tier (100 daily requests) to ensure zero operational cost.
- **Strategic Roadmap:** Managed through a detailed [Product Backlog](doc/po.md#8-product-backlog--task-tracking) with defined Sprints.
- **Quality Assurance:** Every feature is governed by a strict [Definition of Done](doc/po.md#7-definition-of-done-dod).

## ✅ Current Project State
- [x] **Distributed Engine:** API & Seeder worker architecture is live.
- [x] **Smart Quota System:** Automated 100-limit protection is active.
- [x] **Job Tracking:** MongoDB persistence for task statuses is implemented.
