"""
Weekly Project Progress Report Generator

This application fetches timesheet entries from ConnectWise PSA,
organizes them by project, and uses AI to generate
a formatted weekly progress report.

Features:
- Fetch time entries from ConnectWise PSA
- AI summarization (supports OpenAI, Anthropic, or Google Gemini)
- Compare against SharePoint project roadmap (optional)
- Send email via Microsoft Graph API (optional)
- Docker container support for scheduled execution

Usage:
    python main.py                          # Generate report to file
    python main.py --send-email             # Generate and send via email
    python main.py --compare-projects       # Include SharePoint comparison
    python main.py --mode auto              # Fully automated (for cron)
"""

import os
import sys
import argparse
from datetime import datetime
from dotenv import load_dotenv

from connectwise_client import ConnectWiseClient
from ai_summarizer import get_summarizer
from report_generator import ReportGenerator
from graph_client import GraphClient
from sharepoint_projects import SharePointProjects


def generate_html_email(subject: str, body: str) -> str:
    """
    Convert plain text report to HTML for email.
    
    Args:
        subject: Email subject
        body: Plain text body
        
    Returns:
        HTML formatted email body
    """
    # Convert plain text to HTML with proper formatting
    html_body = body.replace("\n", "<br>\n")
    
    # Style the numbered list items
    html_body = html_body.replace("1. Project is On-Track:", "<b>1. Project is On-Track:</b>")
    html_body = html_body.replace("2. ConnectWise Calendar", "<b>2. ConnectWise Calendar</b>")
    html_body = html_body.replace("3. Completed This Week:", "<b>3. Completed This Week:</b>")
    html_body = html_body.replace("4. Planned for Next Week:", "<b>4. Planned for Next Week:</b>")
    
    # Bold the project headers (lines that contain | and end with ))
    lines = html_body.split("<br>\n")
    for i, line in enumerate(lines):
        if "|" in line and ("(" in line or line.strip().endswith(")")):
            # This looks like a project header
            if not line.strip().startswith("<b>"):
                lines[i] = f"<b>{line}</b>"
    
    html_body = "<br>\n".join(lines)
    
    html = f"""
    <html>
    <head>
        <style>
            body {{ font-family: Aptos, Calibri, Helvetica, sans-serif; font-size: 11pt; line-height: 1.5; }}
            b {{ font-weight: bold; }}
        </style>
    </head>
    <body>
        {html_body}
    </body>
    </html>
    """
    return html


def main():
    """Main entry point for the report generator."""
    # Parse command line arguments
    parser = argparse.ArgumentParser(description="Weekly Project Progress Report Generator")
    parser.add_argument("--cli", action="store_true", help="Run in CLI mode (for automated jobs)")
    parser.add_argument("--send-email", action="store_true", help="Send report via email")
    parser.add_argument("--compare-projects", action="store_true", help="Compare with SharePoint roadmap")
    parser.add_argument("--mode", choices=["manual", "auto"], default="manual",
                        help="Execution mode: manual (interactive) or auto (for cron)")
    parser.add_argument("--ai-provider", choices=["gemini", "openai", "anthropic"],
                        help="AI provider to use (auto-detected if not specified)")
    parser.add_argument("--member", help="ConnectWise member identifier (e.g., 'john.doe')")
    parser.add_argument("--test-graph", action="store_true", help="Test Graph API connection")
    args = parser.parse_args()
    
    # If not in CLI mode, show message about web interface
    if not args.cli and not args.test_graph:
        print("=" * 50)
        print("Weekly Project Report Generator")
        print("=" * 50)
        print()
        print("💡 Tip: Use the web interface for easier report generation!")
        print("   Start the web server with: python app.py")
        print()
        print("   Or use CLI mode with: python main.py --cli")
        print()
        return

    # Load environment variables
    load_dotenv()

    # Override member ID if specified in CLI
    if args.member:
        os.environ["CW_MEMBER_ID"] = args.member

    print("=" * 50)
    print("Weekly Project Progress Report Generator")
    print("=" * 50)
    print()

    # Initialize Graph client for SharePoint/email
    graph = GraphClient()

    # Test Graph connection if requested
    if args.test_graph:
        print("Testing Graph API connection...")
        graph.test_connection()
        return

    try:
        # Initialize clients
        print("[1/6] Initializing ConnectWise client...")
        cw_client = ConnectWiseClient()
        print(f"      Connected as member: {cw_client.member_id}")

        print("[2/6] Fetching time entries from last 7 days...")
        entries = cw_client.get_time_entries(days=7)
        print(f"      Found {len(entries)} time entries")

        if not entries:
            print("\n⚠️  No time entries found for the last 7 days.")
            print("    Please check your CW_MEMBER_ID setting.")
            sys.exit(1)

        print("[3/6] Enriching entries with project/ticket details...")
        entries = cw_client.enrich_time_entries(entries)
        print("      Done enriching entries")

        print("[4/6] Organizing entries and generating AI summary...")
        if args.member:
            member_initials = args.member[:2].upper()
        else:
            member_initials = os.getenv("MEMBER_INITIALS", "AN")
        generator = ReportGenerator(member_initials=member_initials)
        grouped = generator.group_entries_by_project(entries)
        print(f"      Found {len(grouped)} unique projects/tickets")

        # Initialize AI summarizer (auto-detects provider based on configured API key)
        summarizer = get_summarizer(preferred_provider=args.ai_provider)
        print(f"      Using AI provider: {summarizer.provider_name}")
        report_date = datetime.now()
        report_body = summarizer.summarize_entries(grouped, report_date.strftime("%Y-%m-%d"))

        # Compare with SharePoint roadmap if requested
        comparison_summary = ""
        if args.compare_projects or args.mode == "auto":
            print("[5/6] Comparing with SharePoint project roadmap...")
            
            # Download Excel from SharePoint
            excel_path = os.path.join(os.path.dirname(__file__), "temp_roadmap.xlsx")
            if graph.download_sharepoint_file(excel_path):
                sp_projects = SharePointProjects(excel_path)
                sp_projects.parse_roadmap()
                
                # Get owner filter from member ID
                owner_filter = os.getenv("CW_MEMBER_ID", "").replace(".", " ")
                
                # Compare
                comparison = sp_projects.compare_with_timesheet(
                    list(grouped.keys()),
                    owner_filter=owner_filter if owner_filter else None
                )
                comparison_summary = sp_projects.format_comparison_summary(comparison)
                print(comparison_summary)
                
                # Clean up temp file
                try:
                    os.remove(excel_path)
                except:
                    pass
            else:
                print("      Skipping comparison (SharePoint not configured)")
        else:
            print("[5/6] Skipping SharePoint comparison (use --compare-projects to enable)")

        print("[6/6] Writing report to file...")
        subject = generator.generate_email_subject(report_date)
        
        # Add comparison summary to report if available
        full_report = report_body
        if comparison_summary:
            full_report += "\n\n" + "=" * 50 + "\nPROJECT COMPARISON\n" + "=" * 50 + comparison_summary
        
        filepath = generator.write_report(subject, full_report, report_date)

        print()
        print("=" * 50)
        print("✅ Report generated successfully!")
        print("=" * 50)
        print()
        print(f"📄 Report saved to: {filepath}")

        # Send email if requested
        if args.send_email or args.mode == "auto":
            print()
            print("📧 Sending email...")
            html_body = generate_html_email(subject, report_body)
            if graph.send_email(subject, html_body, is_html=True):
                print("✅ Email sent successfully!")
            else:
                print("⚠️  Email not sent (Graph API may not be configured)")
        else:
            print()
            print("You can now open this file and copy the contents")
            print("to your email client.")
            print()
            print("💡 Tip: Use --send-email to send automatically via Microsoft Graph")

    except ValueError as e:
        print(f"\n❌ Configuration Error: {e}")
        print("\nPlease check your .env file has all required values:")
        print("  - CW_COMPANY_ID")
        print("  - CW_PUBLIC_KEY")
        print("  - CW_PRIVATE_KEY")
        print("  - CW_MEMBER_ID")
        print("  - GOOGLE_API_KEY or OPENAI_API_KEY or ANTHROPIC_API_KEY")
        sys.exit(1)

    except Exception as e:
        print(f"\n❌ Error: {e}")
        import traceback
        traceback.print_exc()
        sys.exit(1)


if __name__ == "__main__":
    main()
