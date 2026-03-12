# Weekly Project Progress Report Generator

A **Go-based web application** that generates weekly project status reports by fetching timesheet data from ConnectWise PSA, summarizing it with AI, and optionally comparing against a SharePoint project roadmap. Includes automated schedule blocking for the week ahead.

## 🌟 Features

- 🌐 **Web Interface** — Modern dark-themed UI for generating reports from the browser
- 👥 **Member Selection** — Select any team member to generate their report
- 📊 **ConnectWise PSA Integration** — Automatically fetches and groups time entries
- 🤖 **Multi-Provider AI** — Supports Google Gemini, OpenAI, or Anthropic (auto-detects valid key)
- 📋 **Strict Template Format** — Follows your organization's report template exactly
- 📁 **SharePoint Integration** — Compare against your project roadmap Excel file
- 📧 **Email Automation** — Send reports via Microsoft Graph API
- 📅 **Schedule Week Ahead** — AI-estimated schedule blocks pushed to ConnectWise calendar
- 🐳 **Container Support** — Multi-stage Dockerfile with Docker/Podman Compose
- 🔄 **CLI Mode** — Fully featured command-line interface for automation

## 🚀 Quick Start

### Prerequisites

- **Go 1.25+** (for local development)
- ConnectWise PSA API credentials
- AI API key (at least ONE): Google Gemini, OpenAI, or Anthropic
- (Optional) Microsoft Graph credentials for SharePoint / Email
- (Optional) Docker or Podman for container deployment

### Local Development

```bash
# 1. Clone and configure
git clone <repo-url> && cd weekly_project_report
cp .env.example .env
# Edit .env with your credentials

# 2. Build
go build -o weekly-report .

# 3. Start the web server
./weekly-report
# Open http://localhost:5000
```

### CLI Mode (for automation)

```bash
# Generate report for configured member
./weekly-report --cli

# Generate and send via email
./weekly-report --cli --send-email

# Include SharePoint comparison
./weekly-report --cli --compare-projects

# Schedule next week's blocks
./weekly-report --cli --schedule

# Full auto mode (for scheduled jobs)
./weekly-report --cli --mode auto
```

## ⚙️ Configuration

Create a `.env` file (see `.env.example` for all options):

```env
# ConnectWise PSA (Required)
CW_COMPANY_ID=your_company_id
CW_SITE_URL=api-na.myconnectwise.net
CW_PUBLIC_KEY=your_public_key
CW_PRIVATE_KEY=your_private_key
CW_CLIENT_ID=your_client_id
CW_MEMBER_ID=your_member_id

# AI Provider (Required — at least one valid key)
AI_PROVIDER=gemini   # Options: gemini, openai, anthropic
GOOGLE_API_KEY=your_google_api_key

# Microsoft Graph (Optional)
GRAPH_CLIENT_ID=your_azure_app_client_id
GRAPH_CLIENT_SECRET=your_azure_app_client_secret
GRAPH_TENANT_ID=your_azure_tenant_id

# Report Settings
MEMBER_INITIALS=XY
```

### AI Provider Auto-Detection

The app detects which provider to use by checking for valid API keys. Placeholder values like `your_openai_api_key` are ignored. Set `AI_PROVIDER` to override, or let it auto-detect with priority: **Gemini → OpenAI → Anthropic**.

| Provider  | Default Model                | Env Var             |
| --------- | ---------------------------- | ------------------- |
| Gemini    | `gemini-3.1-pro-preview`       | `GOOGLE_API_KEY`    |
| OpenAI    | `gpt-5.1`                    | `OPENAI_API_KEY`    |
| Anthropic | `claude-sonnet-4-20250514`   | `ANTHROPIC_API_KEY` |

## 🐳 Container Deployment

Works with both **Docker** and **Podman**.

### Web Interface Only

```bash
podman-compose up web          # or: docker-compose up web
# Access at http://localhost:5000
```

### Scheduled Reports (Cron)

```bash
podman-compose up -d cron
```

### Both Services

```bash
podman-compose up -d           # Web + Cron
podman-compose down            # Stop all
```

### Manual CLI in Container

```bash
podman-compose run --rm web --cli
podman-compose run --rm web --cli --send-email
```

### Build from Scratch (no cache)

```bash
podman system prune -af
podman build --no-cache -t weekly-report:latest -f Dockerfile .
podman run -d --name weekly-report-web -p 5000:5000 --env-file .env weekly-report:latest
```

## 📅 Schedule Week Ahead

The schedule feature uses AI to estimate time for each project's next step, then proposes calendar blocks for the upcoming week:

1. Check **📅 Schedule Week Ahead** in the web UI, or use `--schedule` in CLI mode
2. AI estimates hours for each project based on completed work
3. Blocks are distributed across Mon–Fri using your ConnectWise shift calendar
4. Stat holidays and PTO/vacation days are automatically skipped
5. Review the proposed schedule, then confirm to create entries in ConnectWise

## 📊 Report Format

```
Company Name | Project Name (#ticket_numbers)

1. Project is On-Track: Yes
   * End Date: January 30
   * Notes: Optional context notes
2. ConnectWise Calendar set for week(s) Ahead: Yes
3. Completed This Week:
   * Summary of accomplishments from timesheet entries
4. Planned for Next Week:
   * AI-inferred next steps based on work context
```

## 🔗 Microsoft Graph Setup

To enable SharePoint and email features:

1. Create an **Azure App Registration** in Azure Portal
2. Add API permissions:
   - `Files.Read.All` (Application) — Read SharePoint files
   - `Mail.Send` (Application) — Send emails
3. Grant admin consent
4. Create a client secret
5. Add credentials to `.env`

## 📁 Project Structure

```
weekly_project_report/
├── main.go                        # Entry point (CLI + web server)
├── internal/
│   ├── ai/
│   │   ├── summarizer.go          # Multi-provider AI summarization
│   │   └── schedule_estimator.go  # AI-based task duration estimation
│   ├── config/
│   │   └── config.go              # Environment configuration
│   ├── connectwise/
│   │   ├── client.go              # ConnectWise PSA API client
│   │   └── schedule.go            # Schedule entries, calendars, holidays
│   ├── graph/
│   │   └── client.go              # Microsoft Graph API client
│   ├── report/
│   │   └── generator.go           # Report formatting and file output
│   ├── schedule/
│   │   └── planner.go             # Week-ahead schedule planner
│   ├── server/
│   │   └── server.go              # HTTP server and API handlers
│   └── sharepoint/
│       └── projects.go            # SharePoint Excel parser
├── web/
│   ├── index.html                 # Web interface
│   ├── css/style.css              # Dark-themed premium UI styles
│   └── js/app.js                  # Frontend application logic
├── Dockerfile                     # Multi-stage Go build
├── docker-compose.yml             # Web + Cron services
├── .env.example                   # Configuration template
├── .github/workflows/
│   └── docker-build.yml           # CI: Go build + container push
└── legacy-python/                 # Archived Python implementation
```

## 🔧 CI/CD

The GitHub Actions workflow (`.github/workflows/docker-build.yml`) runs on push/PR to `main`:

1. **Go Build** — `go vet ./...` and binary compilation
2. **Container Build** — Multi-stage Docker image built and pushed to DockerHub

Required GitHub Secrets:
- `DOCKERHUB_USERNAME`
- `DOCKERHUB_TOKEN`

## License

MIT License — Feel free to modify and use for your own projects.
