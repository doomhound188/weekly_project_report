"""
Report generation service that encapsulates business logic.
"""

import os
from datetime import datetime
from typing import Optional, Dict, Any

from connectwise_client import ConnectWiseClient
from ai_summarizer import get_summarizer
from report_generator import ReportGenerator
from graph_client import GraphClient
from sharepoint_projects import SharePointProjects


class ReportService:
    """Service for generating weekly progress reports."""

    def __init__(self):
        """Initialize the report service with required clients."""
        self.cw_client = None
        self.graph_client = GraphClient()
        
    def _ensure_cw_client(self):
        """Lazy initialization of ConnectWise client."""
        if self.cw_client is None:
            self.cw_client = ConnectWiseClient()
    
    def get_members(self) -> list[dict]:
        """
        Get list of all members from ConnectWise.
        
        Returns:
            List of member dictionaries
        """
        self._ensure_cw_client()
        return self.cw_client.get_members()
    
    def generate_report(
        self,
        member_id: str,
        days: int = 7,
        ai_provider: Optional[str] = None,
        compare_projects: bool = False,
        send_email: bool = False
    ) -> Dict[str, Any]:
        """
        Generate a weekly progress report for a specific member.
        
        Args:
            member_id: ConnectWise member identifier (e.g., "john.doe")
            days: Number of days to look back (default: 7)
            ai_provider: AI provider to use (gemini/openai/anthropic)
            compare_projects: Whether to compare with SharePoint roadmap
            send_email: Whether to send the report via email
            
        Returns:
            Dictionary containing:
                - success: bool
                - report_body: str (the generated report text)
                - report_subject: str
                - comparison_summary: str (optional)
                - email_sent: bool
                - error: str (if success=False)
        """
        try:
            # Initialize ConnectWise client with temporary member ID
            original_member_id = os.getenv("CW_MEMBER_ID")
            os.environ["CW_MEMBER_ID"] = member_id
            
            # Re-initialize client with new member ID
            cw_client = ConnectWiseClient()
            
            # Fetch time entries
            entries = cw_client.get_time_entries(days=days)
            
            if not entries:
                return {
                    "success": False,
                    "error": f"No time entries found for member '{member_id}' in the last {days} days."
                }
            
            # Enrich entries with project/ticket details
            entries = cw_client.enrich_time_entries(entries)
            
            # Group and generate report
            # Use configured initials only if generating for the default member
            if original_member_id and member_id == original_member_id:
                member_initials = os.getenv("MEMBER_INITIALS", member_id[:2].upper())
            else:
                member_initials = member_id[:2].upper()

            generator = ReportGenerator(member_initials=member_initials)
            grouped = generator.group_entries_by_project(entries)
            
            # Initialize AI summarizer
            summarizer = get_summarizer(preferred_provider=ai_provider)
            report_date = datetime.now()
            report_body = summarizer.summarize_entries(grouped, report_date.strftime("%Y-%m-%d"))
            
            # Generate subject
            subject = generator.generate_email_subject(report_date)
            
            # Compare with SharePoint if requested
            comparison_summary = ""
            if compare_projects:
                excel_path = os.path.join(os.path.dirname(__file__), "temp_roadmap.xlsx")
                if self.graph_client.download_sharepoint_file(excel_path):
                    sp_projects = SharePointProjects(excel_path)
                    sp_projects.parse_roadmap()
                    
                    # Get owner filter from member ID
                    owner_filter = member_id.replace(".", " ")
                    
                    # Compare
                    comparison = sp_projects.compare_with_timesheet(
                        list(grouped.keys()),
                        owner_filter=owner_filter if owner_filter else None
                    )
                    comparison_summary = sp_projects.format_comparison_summary(comparison)
                    
                    # Clean up temp file
                    try:
                        os.remove(excel_path)
                    except:
                        pass
            
            # Send email if requested
            email_sent = False
            if send_email:
                from main import generate_html_email
                html_body = generate_html_email(subject, report_body)
                email_sent = self.graph_client.send_email(subject, html_body, is_html=True)
            
            # Restore original member ID
            if original_member_id:
                os.environ["CW_MEMBER_ID"] = original_member_id
            
            return {
                "success": True,
                "report_body": report_body,
                "report_subject": subject,
                "comparison_summary": comparison_summary,
                "email_sent": email_sent,
                "ai_provider": summarizer.provider_name,
                "entry_count": len(entries),
                "project_count": len(grouped)
            }
            
        except Exception as e:
            # Restore original member ID on error
            if original_member_id:
                os.environ["CW_MEMBER_ID"] = original_member_id
                
            return {
                "success": False,
                "error": str(e)
            }
