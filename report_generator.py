"""
Report generator for organizing time entries and creating output files.
"""

import os
from datetime import datetime
from collections import defaultdict


class ReportGenerator:
    """Organizes time entries and generates the final report file."""

    def __init__(self, member_initials: str = "AN"):
        self.member_initials = member_initials
        self.output_dir = os.path.dirname(os.path.abspath(__file__))

    # Patterns to exclude from the report (case-insensitive)
    EXCLUDE_PATTERNS = [
        "meeting",
        "internal",
        "z - internal",
        "admin",
        "pto",
        "vacation",
        "holiday",
        "sick",
    ]

    def group_entries_by_project(self, entries: list[dict]) -> dict[str, list[dict]]:
        """
        Group time entries by project or ticket name, excluding meetings and internal entries.
        
        Args:
            entries: List of time entry dictionaries
            
        Returns:
            Dictionary mapping project names to their entries
        """
        grouped = defaultdict(list)

        for entry in entries:
            # Determine the project/ticket name
            project_name = self._get_project_name(entry)
            
            # Skip if this is a meeting or internal entry
            if self._should_exclude(project_name):
                continue
                
            grouped[project_name].append(entry)

        return dict(grouped)

    def _should_exclude(self, project_name: str) -> bool:
        """
        Check if a project should be excluded from the report.
        
        Args:
            project_name: The project name to check
            
        Returns:
            True if the project should be excluded, False otherwise
        """
        name_lower = project_name.lower()
        return any(pattern in name_lower for pattern in self.EXCLUDE_PATTERNS)

    def _get_project_name(self, entry: dict) -> str:
        """
        Extract the project or ticket name from a time entry.
        
        Args:
            entry: Time entry dictionary
            
        Returns:
            Project/ticket name string
        """
        # Try to get project name first
        if entry.get("_project_details") and entry["_project_details"].get("name"):
            return entry["_project_details"]["name"]
        
        if entry.get("project") and entry["project"].get("name"):
            return entry["project"]["name"]

        # Fall back to ticket summary
        if entry.get("_ticket_details") and entry["_ticket_details"].get("summary"):
            return entry["_ticket_details"]["summary"]
        
        if entry.get("ticket") and entry["ticket"].get("summary"):
            return entry["ticket"]["summary"]

        # Last resort: use work type or generic name
        if entry.get("workType") and entry["workType"].get("name"):
            return f"General: {entry['workType']['name']}"

        return "Uncategorized Work"

    def generate_email_subject(self, report_date: datetime) -> str:
        """
        Generate the email subject line.
        
        Args:
            report_date: The date for the report
            
        Returns:
            Formatted email subject string
        """
        date_str = report_date.strftime("%Y-%m-%d")
        return f"{self.member_initials} | Weekly Project Progress Report | {date_str}"

    def write_report(self, subject: str, body: str, report_date: datetime) -> str:
        """
        Write the report to a text file.
        
        Args:
            subject: Email subject line
            body: Report body content
            report_date: The date for the report
            
        Returns:
            Path to the generated report file
        """
        date_str = report_date.strftime("%Y-%m-%d")
        filename = f"weekly_report_{date_str}.txt"
        filepath = os.path.join(self.output_dir, filename)

        with open(filepath, "w", encoding="utf-8") as f:
            f.write("=" * 60 + "\n")
            f.write("WEEKLY PROJECT PROGRESS REPORT\n")
            f.write("=" * 60 + "\n\n")
            f.write(f"Subject: {subject}\n")
            f.write("-" * 60 + "\n\n")
            f.write("Body:\n\n")
            f.write(body)
            f.write("\n\n" + "=" * 60 + "\n")
            f.write("END OF REPORT\n")
            f.write("=" * 60 + "\n")

        return filepath
