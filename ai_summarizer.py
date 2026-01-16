"""
AI Summarizer - Supports OpenAI, Anthropic, and Google Gemini.
Automatically selects provider based on configured API key.
"""

import os
from abc import ABC, abstractmethod
from typing import Optional


# Strict template for report formatting
REPORT_TEMPLATE = """You are generating a weekly project progress report from timesheet entries.

For EACH project, you MUST follow this EXACT format structure. Use a numbered list (1., 2., 3., 4.) with nested bullet points (*):

{Company Name} | {Project Name} (#{ticket_numbers})

1. Project is On-Track: Yes
   * End Date: {date like "January 16" or "Ongoing"}
   * Notes: {brief context - ONLY include this bullet if there are relevant notes}
2. ConnectWise Calendar set for week(s) Ahead: Yes
3. Completed This Week:
   * {Summarize accomplishments as bullet points. If no work done, write "No updates; focus remained on other priorities."}
4. Planned for Next Week:
   * {Infer next steps as bullet points. If work complete, write "N/A (task completed)."}

CRITICAL FORMATTING RULES:
1. The project header line should include company name, project name, and ticket numbers - NO BOLD, NO ASTERISKS
2. Always use numbered list 1-4 for the main items
3. Use nested bullet points (*) for sub-items under each numbered item
4. End Date should be formatted as "January 16" not "2026-01-16"
5. Notes bullet is OPTIONAL - only include if there's meaningful context to add
6. ConnectWise Calendar is typically "Yes" unless it's a completed project
7. Group all time entries by project and summarize into cohesive bullet points
8. Use professional, concise language in past tense for "Completed This Week"
9. For "Planned for Next Week", infer logical next steps based on the work context
10. Separate each project section with a blank line
11. ABSOLUTELY NO MARKDOWN - no ** symbols, no __ symbols, no # symbols - this is plain text for Outlook email
12. Output ONLY plain text that can be directly pasted into an email

EXAMPLE OUTPUT (follow this EXACT structure with NO markdown formatting):
Infinity Network Solutions | Automation: Teams Bot (InfiniBot) (#209066 / #209067)

1. Project is On-Track: Yes
   * End Date: January 16
   * Notes: Progress on building and deploying internal Teams bot for automation integration.
2. ConnectWise Calendar set for week(s) Ahead: Yes
3. Completed This Week:
   * Built Azure bot resources, validated manifest, resolved identity mismatch, and published bot for testing.
4. Planned for Next Week:
   * Finalize proactive messaging integration with Rewst and document deployment process.
"""


class BaseSummarizer(ABC):
    """Abstract base class for AI summarizers."""

    @abstractmethod
    def summarize_entries(self, grouped_entries: dict[str, list[dict]], report_date: str) -> str:
        """Generate summary from grouped time entries."""
        pass

    def _build_prompt(self, grouped_entries: dict[str, list[dict]], report_date: str) -> str:
        """Build the prompt with all entries."""
        prompt = REPORT_TEMPLATE + "\n\nHere are the timesheet entries to summarize:\n\n"

        for project_name, entries in grouped_entries.items():
            prompt += f"--- PROJECT: {project_name} ---\n"

            for entry in entries:
                date = entry.get("dateEntered", "Unknown date")
                hours = entry.get("actualHours", 0)
                notes = entry.get("notes", "No notes provided")

                # Get company name from various sources
                company = "Unknown Company"
                if entry.get("company") and entry["company"].get("name"):
                    company = entry["company"]["name"]
                elif entry.get("_ticket_details") and entry["_ticket_details"].get("company"):
                    company = entry["_ticket_details"]["company"].get("name", company)
                elif entry.get("_project_details") and entry["_project_details"].get("company"):
                    company = entry["_project_details"]["company"].get("name", company)

                # Get ticket/project IDs
                ticket_id = ""
                if entry.get("ticket") and entry["ticket"].get("id"):
                    ticket_id = f"#{entry['ticket']['id']}"
                elif entry.get("_ticket_details") and entry["_ticket_details"].get("id"):
                    ticket_id = f"#{entry['_ticket_details']['id']}"

                # Get end date if available
                end_date = "Ongoing"
                if entry.get("_project_details") and entry["_project_details"].get("estimatedEnd"):
                    end_date = entry["_project_details"]["estimatedEnd"]

                prompt += f"  Company: {company}\n"
                prompt += f"  Date: {date}\n"
                prompt += f"  Hours: {hours}\n"
                prompt += f"  Ticket: {ticket_id}\n"
                prompt += f"  End Date: {end_date}\n"
                prompt += f"  Notes: {notes}\n\n"

            prompt += "\n"

        prompt += f"\nPlease generate the weekly report for the week ending {report_date}."
        return prompt


class GeminiSummarizer(BaseSummarizer):
    """Google Gemini summarizer."""
    
    DEFAULT_MODEL = "gemini-3-pro-preview"

    def __init__(self, model: Optional[str] = None):
        import google.generativeai as genai

        api_key = os.getenv("GOOGLE_API_KEY")
        if not api_key:
            raise ValueError("Missing GOOGLE_API_KEY")

        # Use provided model, env var, or default
        model = model or os.getenv("AI_MODEL") or self.DEFAULT_MODEL
        
        genai.configure(api_key=api_key)
        self.model = genai.GenerativeModel(model)
        self.provider_name = f"Google Gemini ({model})"

    def summarize_entries(self, grouped_entries: dict[str, list[dict]], report_date: str) -> str:
        prompt = self._build_prompt(grouped_entries, report_date)
        response = self.model.generate_content(prompt)
        return response.text


class OpenAISummarizer(BaseSummarizer):
    """OpenAI summarizer (GPT-4, GPT-5, etc.)."""
    
    DEFAULT_MODEL = "gpt-5.1"

    def __init__(self, model: Optional[str] = None):
        from openai import OpenAI

        api_key = os.getenv("OPENAI_API_KEY")
        if not api_key:
            raise ValueError("Missing OPENAI_API_KEY")

        # Use provided model, env var, or default
        model = model or os.getenv("AI_MODEL") or self.DEFAULT_MODEL
        
        self.client = OpenAI(api_key=api_key)
        self.model_name = model
        self.provider_name = f"OpenAI ({model})"

    def summarize_entries(self, grouped_entries: dict[str, list[dict]], report_date: str) -> str:
        prompt = self._build_prompt(grouped_entries, report_date)

        response = self.client.chat.completions.create(
            model=self.model_name,
            messages=[
                {"role": "system", "content": "You are a professional report generator."},
                {"role": "user", "content": prompt}
            ],
            temperature=0.7
        )

        return response.choices[0].message.content


class AnthropicSummarizer(BaseSummarizer):
    """Anthropic Claude summarizer."""
    
    DEFAULT_MODEL = "claude-sonnet-4-20250514"

    def __init__(self, model: Optional[str] = None):
        import anthropic

        api_key = os.getenv("ANTHROPIC_API_KEY")
        if not api_key:
            raise ValueError("Missing ANTHROPIC_API_KEY")

        # Use provided model, env var, or default
        model = model or os.getenv("AI_MODEL") or self.DEFAULT_MODEL
        
        self.client = anthropic.Anthropic(api_key=api_key)
        self.model_name = model
        self.provider_name = f"Anthropic ({model})"

    def summarize_entries(self, grouped_entries: dict[str, list[dict]], report_date: str) -> str:
        prompt = self._build_prompt(grouped_entries, report_date)

        response = self.client.messages.create(
            model=self.model_name,
            max_tokens=4096,
            messages=[
                {"role": "user", "content": prompt}
            ]
        )

        return response.content[0].text


def get_summarizer(preferred_provider: Optional[str] = None) -> BaseSummarizer:
    """
    Get the appropriate summarizer based on configured API keys.

    Args:
        preferred_provider: Optional preferred provider ("gemini", "openai", "anthropic")

    Returns:
        Configured summarizer instance

    Raises:
        ValueError: If no AI API key is configured
    """
    # Map of providers and their required env vars
    providers = {
        "gemini": ("GOOGLE_API_KEY", GeminiSummarizer),
        "openai": ("OPENAI_API_KEY", OpenAISummarizer),
        "anthropic": ("ANTHROPIC_API_KEY", AnthropicSummarizer),
    }

    # Check preferred provider first
    if preferred_provider:
        preferred_provider = preferred_provider.lower()
        if preferred_provider in providers:
            env_var, summarizer_class = providers[preferred_provider]
            if os.getenv(env_var):
                return summarizer_class()

    # Check configured AI provider env var
    configured = os.getenv("AI_PROVIDER", "").lower()
    if configured in providers:
        env_var, summarizer_class = providers[configured]
        if os.getenv(env_var):
            return summarizer_class()

    # Auto-detect based on available API keys (priority order)
    priority_order = ["anthropic", "openai", "gemini"]

    for provider in priority_order:
        env_var, summarizer_class = providers[provider]
        if os.getenv(env_var):
            return summarizer_class()

    # No provider configured
    raise ValueError(
        "No AI API key configured. Please set one of:\n"
        "  - GOOGLE_API_KEY (Google Gemini)\n"
        "  - OPENAI_API_KEY (OpenAI GPT)\n"
        "  - ANTHROPIC_API_KEY (Anthropic Claude)"
    )
