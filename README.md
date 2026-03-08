# 🚀 Gopher-Search: Automated Niche Lead Discovery

A distributed, high-concurrency Go system designed to automate niche data extraction while strictly managing API costs through intelligent quota governance.

---

## 👔 Product Management Highlights
This project demonstrates professional **Product Owner** methodologies:
- **Cost-Efficiency First:** Built-in safeguards to utilize the Google Search API free-tier (100 daily requests) with zero operational cost.
- **Strategic Planning:** Managed via a detailed [Product Backlog](doc/po.md#8-product-backlog--task-tracking) and prioritized sprints.
- **Quality Assurance:** All features must meet a strict [Definition of Done](doc/po.md#7-definition-of-done-dod) before deployment.

## ✅ Current Project State
- [x] **Distributed Engine:** Microservice-style architecture with API & Seeder workers.
- [x] **Smart Quota System:** Automated protection against exceeding daily API limits.
- [x] **Job Pipeline:** Full persistence of task states (`pending`, `processing`, `done`) in MongoDB.

## 🛠 Tech Stack
- **Backend:** Golang (Concurrency-focused worker pattern)
- **Database:** MongoDB (Flexible task storage & Quota tracking)
- **Log Management:** Zap + Lumberjack (Structured logging with rotation)
- **Infrastructure:** Docker, Docker Compose, Nginx

## 🏗 System Architecture
The project is split into two specialized services to ensure scalability:
1. **API Service:** Handles user requests, manages the job queue, and enforces quota logic.
2. **Seeder Service:** The "Worker" that performs live Google searches and processes data.

## 📂 Documentation
- [Product Requirement Document (PRD)](doc/po.md) - Deep dive into vision, user stories, and roadmap.
- [API Logs](api.log) - Real-time audit trails of system operations.

## 🚀 Getting Started
1. Clone the repository.
2. Configure your `.env` with Google API credentials.
3. Run `docker-compose up --build`.
