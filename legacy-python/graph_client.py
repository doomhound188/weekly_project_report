"""
Microsoft Graph API client for SharePoint file access and email sending.
"""

import os
import base64
from typing import Optional
import requests


class GraphClient:
    """Client for Microsoft Graph API operations."""

    GRAPH_BASE_URL = "https://graph.microsoft.com/v1.0"

    def __init__(self):
        self.client_id = os.getenv("GRAPH_CLIENT_ID")
        self.client_secret = os.getenv("GRAPH_CLIENT_SECRET")
        self.tenant_id = os.getenv("GRAPH_TENANT_ID")
        self.site_name = os.getenv("SHAREPOINT_SITE_NAME", "Clients")
        self.file_path = os.getenv("SHAREPOINT_FILE_PATH", "INS Project Roadmap 2025.xlsx")
        self.email_sender = os.getenv("EMAIL_SENDER", "")
        self.email_recipient = os.getenv("EMAIL_RECIPIENT", "")

        self._access_token = None
        self._site_id = None

    @property
    def is_configured(self) -> bool:
        """Check if Graph API credentials are configured."""
        return all([self.client_id, self.client_secret, self.tenant_id])

    def _get_access_token(self) -> str:
        """
        Get an access token using client credentials flow.
        
        Returns:
            Access token string
        """
        if self._access_token:
            return self._access_token

        token_url = f"https://login.microsoftonline.com/{self.tenant_id}/oauth2/v2.0/token"

        data = {
            "client_id": self.client_id,
            "client_secret": self.client_secret,
            "scope": "https://graph.microsoft.com/.default",
            "grant_type": "client_credentials"
        }

        response = requests.post(token_url, data=data)
        response.raise_for_status()

        self._access_token = response.json()["access_token"]
        return self._access_token

    def _get_headers(self) -> dict:
        """Get authorization headers for Graph API requests."""
        return {
            "Authorization": f"Bearer {self._get_access_token()}",
            "Content-Type": "application/json"
        }

    def _get_site_id(self) -> str:
        """
        Get the SharePoint site ID.
        
        Returns:
            Site ID string
        """
        if self._site_id:
            return self._site_id

        # Get site by name
        url = f"{self.GRAPH_BASE_URL}/sites/infinityns.sharepoint.com:/sites/{self.site_name}"
        response = requests.get(url, headers=self._get_headers())
        response.raise_for_status()

        self._site_id = response.json()["id"]
        return self._site_id

    def download_sharepoint_file(self, output_path: str) -> bool:
        """
        Download a file from SharePoint.
        
        Args:
            output_path: Local path to save the downloaded file
            
        Returns:
            True if successful, False otherwise
        """
        if not self.is_configured:
            print("⚠️  Graph API not configured, skipping SharePoint download")
            return False

        try:
            site_id = self._get_site_id()

            # Search for the file in the site's drive
            # First, get the default drive
            drive_url = f"{self.GRAPH_BASE_URL}/sites/{site_id}/drive"
            drive_response = requests.get(drive_url, headers=self._get_headers())
            drive_response.raise_for_status()
            drive_id = drive_response.json()["id"]

            # Search for the file
            search_url = f"{self.GRAPH_BASE_URL}/drives/{drive_id}/root/search(q='{self.file_path}')"
            search_response = requests.get(search_url, headers=self._get_headers())
            search_response.raise_for_status()

            items = search_response.json().get("value", [])
            if not items:
                print(f"⚠️  File not found: {self.file_path}")
                return False

            # Get the download URL for the first matching file
            file_item = items[0]
            download_url = file_item.get("@microsoft.graph.downloadUrl")

            if not download_url:
                # Get download URL via item endpoint
                item_url = f"{self.GRAPH_BASE_URL}/drives/{drive_id}/items/{file_item['id']}/content"
                download_response = requests.get(item_url, headers=self._get_headers(), allow_redirects=True)
                download_response.raise_for_status()
                
                with open(output_path, "wb") as f:
                    f.write(download_response.content)
            else:
                # Direct download
                download_response = requests.get(download_url)
                download_response.raise_for_status()
                
                with open(output_path, "wb") as f:
                    f.write(download_response.content)

            print(f"✅ Downloaded SharePoint file to: {output_path}")
            return True

        except requests.exceptions.HTTPError as e:
            print(f"❌ Graph API error: {e}")
            if hasattr(e, 'response') and e.response is not None:
                print(f"   Response: {e.response.text[:500]}")
            return False
        except Exception as e:
            print(f"❌ Error downloading SharePoint file: {e}")
            return False

    def send_email(self, subject: str, body: str, is_html: bool = True) -> bool:
        """
        Send an email using Microsoft Graph API.
        
        Args:
            subject: Email subject
            body: Email body content
            is_html: Whether the body is HTML formatted
            
        Returns:
            True if successful, False otherwise
        """
        if not self.is_configured:
            print("⚠️  Graph API not configured, skipping email send")
            return False

        if not self.email_sender or not self.email_recipient:
            print("⚠️  Email sender/recipient not configured")
            return False

        try:
            url = f"{self.GRAPH_BASE_URL}/users/{self.email_sender}/sendMail"

            email_data = {
                "message": {
                    "subject": subject,
                    "body": {
                        "contentType": "HTML" if is_html else "Text",
                        "content": body
                    },
                    "toRecipients": [
                        {
                            "emailAddress": {
                                "address": self.email_recipient
                            }
                        }
                    ]
                },
                "saveToSentItems": True
            }

            response = requests.post(url, headers=self._get_headers(), json=email_data)
            response.raise_for_status()

            print(f"✅ Email sent successfully to: {self.email_recipient}")
            return True

        except requests.exceptions.HTTPError as e:
            print(f"❌ Failed to send email: {e}")
            if hasattr(e, 'response') and e.response is not None:
                print(f"   Response: {e.response.text[:500]}")
            return False
        except Exception as e:
            print(f"❌ Error sending email: {e}")
            return False

    def test_connection(self) -> bool:
        """
        Test the Graph API connection.
        
        Returns:
            True if connection is successful
        """
        if not self.is_configured:
            print("⚠️  Graph API credentials not configured")
            return False

        try:
            # Try to get an access token
            self._get_access_token()
            print("✅ Graph API authentication successful")
            return True
        except Exception as e:
            print(f"❌ Graph API authentication failed: {e}")
            return False
