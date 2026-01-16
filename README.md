# Weekly Project Progress Report Generator

Automate your weekly project status reports by fetching timesheet data from ConnectWise PSA, organizing it with AI, and optionally comparing against your SharePoint project roadmap.

## Features

- 📊 **ConnectWise PSA Integration** - Automatically fetches your last 7 days of time entries
- 🤖 **Multi-Provider AI** - Supports OpenAI (GPT-4), Anthropic (Claude), or Google Gemini
- 📋 **Strict Template Format** - Follows your organization's report template exactly
- 📁 **SharePoint Integration** - Compare against your project roadmap Excel file
- 📧 **Email Automation** - Send reports via Microsoft Graph API
- 🐳 **Docker Support** - Schedule automatic Friday 12:00 PM reports

## Quick Start

### Prerequisites

- Python 3.9+
- ConnectWise PSA API credentials
- AI API key (ONE of the following):
  - Google Gemini API key
  - OpenAI API key
  - Anthropic API key

### Installation

```bash
# Clone or download the project
cd weekly_project_report

# Install dependencies
pip install -r requirements.txt

# Configure environment
cp .env.example .env
# Edit .env with your credentials
```

### Usage

```bash
# Generate report to file
python main.py

# Generate and send via email
python main.py --send-email

# Include SharePoint project comparison
python main.py --compare-projects

# Full auto mode (for scheduled jobs)
python main.py --mode auto

# Use a specific AI provider
python main.py --ai-provider openai
```

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

## Docker Deployment

Run the report generator automatically every Friday at 12:00 PM:

```bash
# Build the container
docker-compose build

# Start the scheduler
docker-compose up -d

# View logs
docker-compose logs -f

# Run manually
docker-compose run --rm weekly-report python main.py
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

## Project Structure

```
weekly_project_report/
├── main.py                 # Entry point with CLI
├── connectwise_client.py   # ConnectWise PSA API client
├── ai_summarizer.py        # Multi-provider AI (OpenAI/Anthropic/Gemini)
├── gemini_summarizer.py    # Legacy Gemini-only (deprecated)
├── report_generator.py     # Report formatting and output
├── graph_client.py         # Microsoft Graph API client
├── sharepoint_projects.py  # SharePoint Excel parser
├── Dockerfile              # Container build
├── docker-compose.yml      # Container orchestration
├── entrypoint.sh           # Container startup
├── crontab                 # Friday 12:00 PM schedule
├── requirements.txt        # Python dependencies
└── .env.example            # Configuration template
```

## License

MIT License - Feel free to modify and use for your own projects.
