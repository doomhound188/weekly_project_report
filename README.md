# Weekly Project Progress Report Generator

**Web-based interface** for generating weekly project status reports by fetching timesheet data from ConnectWise PSA, organizing it with AI, and optionally comparing against your SharePoint project roadmap.

## 🌟 Features

- 🌐 **Web Interface** - User-friendly browser interface for any team member
- 👥 **Member Selection** - Select any team member to generate their report
- 📊 **ConnectWise PSA Integration** - Automatically fetches time entries
- 🤖 **Multi-Provider AI** - Supports OpenAI (GPT-4), Anthropic (Claude), or Google Gemini
- 📋 **Strict Template Format** - Follows your organization's report template exactly
- 📁 **SharePoint Integration** - Compare against your project roadmap Excel file
- 📧 **Email Automation** - Send reports via Microsoft Graph API
- 🐳 **Docker Support** - Easy deployment with both web and scheduled modes
- 🔄 **CLI Mode** - Backward compatible for automation and cron jobs

## 🚀 Quick Start

### Web Interface (Recommended)

```bash
# 1. Install dependencies
pip install -r requirements.txt

# 2. Configure environment
cp .env.example .env
# Edit .env with your credentials

# 3. Start the web server
python app.py

# 4. Open browser to http://localhost:5000
```

### CLI Mode (for automation)

```bash
# Generate report for configured user
python main.py --cli

# Generate and send via email
python main.py --cli --send-email

# Include SharePoint comparison
python main.py --cli --compare-projects

# Full auto mode (for scheduled jobs)
python main.py --cli --mode auto
```

## 📋 Prerequisites

- Python 3.9+
- ConnectWise PSA API credentials
- AI API key (ONE of the following):
  - Google Gemini API key
  - OpenAI API key
  - Anthropic API key
- (Optional) Microsoft Graph API credentials for SharePoint/Email

## Configuration

Create a `.env` file with the following:

```env
# ConnectWise PSA (Required)
CW_COMPANY_ID=your_company_id
CW_SITE_URL=api-na.myconnectwise.net
CW_PUBLIC_KEY=your_public_key
CW_PRIVATE_KEY=your_private_key
CW_CLIENT_ID=your_client_id
CW_MEMBER_ID=your_member_id

# AI Provider (Required - configure ONE)
# App auto-detects based on which key is set
AI_PROVIDER=gemini  # Options: gemini, openai, anthropic
AI_MODEL=           # Optional: override default model

GOOGLE_API_KEY=your_google_api_key      # Default: gemini-3-pro-preview
OPENAI_API_KEY=your_openai_api_key      # Default: gpt-5.1
ANTHROPIC_API_KEY=your_anthropic_api_key # Default: claude-sonnet-4-20250514

# Microsoft Graph (Optional - for SharePoint/Email)
GRAPH_CLIENT_ID=your_azure_app_client_id
GRAPH_CLIENT_SECRET=your_azure_app_client_secret
GRAPH_TENANT_ID=your_azure_tenant_id

# SharePoint (Optional)
SHAREPOINT_SITE_NAME=YourSiteName
SHAREPOINT_FILE_PATH=Project Roadmap.xlsx

# Email (Optional)
EMAIL_RECIPIENT=team@yourcompany.com
EMAIL_SENDER=user@yourcompany.com

# Report Settings
MEMBER_INITIALS=XY
```

## 🐳 Docker Deployment

### Web Interface Only

```bash
# Build and start the web interface
docker-compose up web

# Access at http://localhost:5000

# Run in background
docker-compose up -d web

# View logs
docker-compose logs -f web
```

### Scheduled Reports (Cron)

Run the report generator automatically every Friday at 12:00 PM:

```bash
# Start the cron scheduler
docker-compose up -d cron

# View logs
docker-compose logs -f cron
```

### Both Services

```bash
# Run both web interface and scheduled reports
docker-compose up -d

# Stop all services
docker-compose down
```

### Manual CLI Execution in Docker

```bash
# Run report generation manually
docker-compose run --rm web python main.py --cli

# Run with email
docker-compose run --rm web python main.py --cli --send-email
```

## Report Format

The generated report follows this template:

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

## Excluded Entries

The following types of time entries are automatically filtered out:
- Internal meetings
- Admin time
- PTO / Vacation / Holiday / Sick time

Customize in `report_generator.py` → `EXCLUDE_PATTERNS`

## Microsoft Graph Setup

To enable SharePoint and email features:

1. Create an Azure App Registration in Azure Portal
2. Add API permissions:
   - `Files.Read.All` (Application) - Read SharePoint files
   - `Mail.Send` (Application) - Send emails
3. Grant admin consent
4. Create a client secret
5. Add credentials to `.env`

## 📁 Project Structure

```
weekly_project_report/
├── app.py                  # Flask web application
├── main.py                 # CLI entry point
├── services/
│   ├── __init__.py
│   └── report_service.py   # Business logic layer
├── connectwise_client.py   # ConnectWise PSA API client
├── ai_summarizer.py        # Multi-provider AI (OpenAI/Anthropic/Gemini)
├── report_generator.py     # Report formatting and output
├── graph_client.py         # Microsoft Graph API client
├── sharepoint_projects.py  # SharePoint Excel parser
├── static/
│   └── css/
│       └── style.css       # Modern UI styles
├── templates/
│   └── index.html          # Web interface template
├── Dockerfile              # Container build
├── docker-compose.yml      # Container orchestration (web + cron)
├── entrypoint.sh           # Container startup
├── crontab                 # Friday 12:00 PM schedule
├── requirements.txt        # Python dependencies
└── .env.example            # Configuration template
```

## License

MIT License - Feel free to modify and use for your own projects.
