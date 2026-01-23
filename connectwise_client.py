"""
ConnectWise PSA API Client for fetching time entries and project details.
"""

import os
import base64
from datetime import datetime, timedelta
from typing import Optional
import requests


class ConnectWiseClient:
    """Client for interacting with ConnectWise PSA REST API."""

    def __init__(self):
        self.company_id = os.getenv("CW_COMPANY_ID")
        self.site_url = os.getenv("CW_SITE_URL", "api-na.myconnectwise.net")
        self.public_key = os.getenv("CW_PUBLIC_KEY")
        self.private_key = os.getenv("CW_PRIVATE_KEY")
        self.client_id = os.getenv("CW_CLIENT_ID")
        self.member_id = os.getenv("CW_MEMBER_ID")

        if not all([self.company_id, self.public_key, self.private_key, self.member_id]):
            raise ValueError("Missing required ConnectWise credentials in environment variables")

        self.base_url = f"https://{self.site_url}/v4_6_release/apis/3.0"
        self.headers = self._build_headers()

    def _build_headers(self) -> dict:
        """Build authentication headers for API requests."""
        auth_string = f"{self.company_id}+{self.public_key}:{self.private_key}"
        auth_bytes = base64.b64encode(auth_string.encode()).decode()

        headers = {
            "Authorization": f"Basic {auth_bytes}",
            "Content-Type": "application/json",
        }

        if self.client_id:
            headers["clientId"] = self.client_id

        return headers

    def get_time_entries(self, days: int = 7) -> list[dict]:
        """
        Fetch time entries for the specified member from the last N days.
        
        Args:
            days: Number of days to look back (default: 7)
            
        Returns:
            List of time entry dictionaries
        """
        start_date = (datetime.now() - timedelta(days=days)).strftime("%Y-%m-%d")
        
        # Build conditions filter
        conditions = f"member/identifier='{self.member_id}' and dateEntered>=[{start_date}]"
        
        all_entries = []
        page = 1
        page_size = 100

        while True:
            params = {
                "conditions": conditions,
                "page": page,
                "pageSize": page_size,
                "orderBy": "dateEntered desc"
            }

            response = requests.get(
                f"{self.base_url}/time/entries",
                headers=self.headers,
                params=params
            )
            response.raise_for_status()
            
            entries = response.json()
            if not entries:
                break
                
            all_entries.extend(entries)
            
            if len(entries) < page_size:
                break
                
            page += 1

        return all_entries

    def get_project(self, project_id: int) -> Optional[dict]:
        """
        Fetch project details by ID.
        
        Args:
            project_id: The ConnectWise project ID
            
        Returns:
            Project dictionary or None if not found
        """
        try:
            response = requests.get(
                f"{self.base_url}/project/projects/{project_id}",
                headers=self.headers
            )
            response.raise_for_status()
            return response.json()
        except requests.exceptions.HTTPError:
            return None

    def get_ticket(self, ticket_id: int) -> Optional[dict]:
        """
        Fetch service ticket details by ID.
        
        Args:
            ticket_id: The ConnectWise ticket ID
            
        Returns:
            Ticket dictionary or None if not found
        """
        try:
            response = requests.get(
                f"{self.base_url}/service/tickets/{ticket_id}",
                headers=self.headers
            )
            response.raise_for_status()
            return response.json()
        except requests.exceptions.HTTPError:
            return None

    def enrich_time_entries(self, entries: list[dict]) -> list[dict]:
        """
        Enrich time entries with project/ticket details.
        
        Args:
            entries: List of time entry dictionaries
            
        Returns:
            Enriched time entries with project/ticket information
        """
        # Cache for projects and tickets to avoid duplicate API calls
        project_cache = {}
        ticket_cache = {}

        for entry in entries:
            # Handle project-based time entries
            if entry.get("project") and entry["project"].get("id"):
                project_id = entry["project"]["id"]
                if project_id not in project_cache:
                    project_cache[project_id] = self.get_project(project_id)
                entry["_project_details"] = project_cache[project_id]

            # Handle ticket-based time entries
            if entry.get("ticket") and entry["ticket"].get("id"):
                ticket_id = entry["ticket"]["id"]
                if ticket_id not in ticket_cache:
                    ticket_cache[ticket_id] = self.get_ticket(ticket_id)
                entry["_ticket_details"] = ticket_cache[ticket_id]

        return entries

    def get_members(self) -> list[dict]:
        """
        Fetch all members from ConnectWise, excluding API accounts.
        
        Returns:
            List of member dictionaries with id, identifier, name, and email
        """
        all_members = []
        page = 1
        page_size = 100

        while True:
            params = {
                "page": page,
                "pageSize": page_size,
                "orderBy": "identifier asc"
            }

            response = requests.get(
                f"{self.base_url}/system/members",
                headers=self.headers,
                params=params
            )
            response.raise_for_status()
            
            members = response.json()
            if not members:
                break
            
            # Filter out API accounts (licenseClass = "A")
            for member in members:
                if member.get("licenseClass") != "A" and member.get("inactiveFlag") != True:
                    all_members.append({
                        "id": member.get("id"),
                        "identifier": member.get("identifier"),
                        "firstName": member.get("firstName", ""),
                        "lastName": member.get("lastName", ""),
                        "name": f"{member.get('firstName', '')} {member.get('lastName', '')}".strip(),
                        "email": member.get("officeEmail", "")
                    })
            
            if len(members) < page_size:
                break
                
            page += 1

        return all_members

