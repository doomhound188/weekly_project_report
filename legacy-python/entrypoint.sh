#!/bin/bash

echo "=============================================="
echo "Weekly Report Generator - Container Started"
echo "=============================================="
echo "Timezone: $(date +%Z)"
echo "Current time: $(date)"
echo ""
echo "Cron schedule: Every Friday at 12:00 PM"
echo ""

# Print environment status (without secrets)
echo "Configuration status:"
echo "  - CW_COMPANY_ID: ${CW_COMPANY_ID:+configured}"
echo "  - CW_MEMBER_ID: ${CW_MEMBER_ID:+configured}"
echo "  - GOOGLE_API_KEY: ${GOOGLE_API_KEY:+configured}"
echo "  - GRAPH_CLIENT_ID: ${GRAPH_CLIENT_ID:+configured}"
echo "  - EMAIL_RECIPIENT: ${EMAIL_RECIPIENT:-not set}"
echo ""

# Export environment variables for cron
printenv | grep -E '^(CW_|GOOGLE_|GRAPH_|EMAIL_|SHAREPOINT_|MEMBER_)' > /app/.env.cron

# Modify the cron job to source environment
sed -i 's|python /app/main.py|set -a; source /app/.env.cron; set +a; python /app/main.py|' /etc/cron.d/weekly-report

# Test the configuration on startup
echo "Testing configuration..."
cd /app
python -c "
from dotenv import load_dotenv
load_dotenv()
from connectwise_client import ConnectWiseClient
from graph_client import GraphClient

try:
    cw = ConnectWiseClient()
    print('✅ ConnectWise client initialized')
except Exception as e:
    print(f'⚠️  ConnectWise: {e}')

graph = GraphClient()
if graph.is_configured:
    print('✅ Graph API configured')
else:
    print('⚠️  Graph API not configured (email/SharePoint disabled)')
"

echo ""
echo "Starting cron daemon..."
echo "=============================================="

# Start cron in foreground
cron -f
