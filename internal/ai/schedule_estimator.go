package ai

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/weekly-report/internal/config"
	"github.com/weekly-report/internal/connectwise"
)

// TaskEstimate represents an AI-estimated time block for a project's next step.
type TaskEstimate struct {
	Project        string  `json:"project"`
	Company        string  `json:"company"`
	TicketID       int     `json:"ticket_id"`
	NextStep       string  `json:"next_step"`
	EstimatedHours float64 `json:"estimated_hours"`
}

const estimatePrompt = `You are a scheduling assistant for an MSP (managed service provider) technician.

Given the following weekly report data showing what was completed this week and what is planned for next week, estimate how many HOURS each "Planned for Next Week" task will take.

ESTIMATION RULES:
- Be realistic. Most MSP project tasks take 1-4 hours per session.
- Consider the complexity from the "Completed This Week" context:
  * Simple follow-up tasks (documentation, client communication): 0.5-1 hour
  * Configuration or deployment tasks: 1-2 hours
  * Troubleshooting or investigation tasks: 2-3 hours
  * Complex implementation or migration tasks: 3-4 hours
  * Large multi-step builds or overhauls: 4-6 hours
- Round to the nearest 0.5 hours.
- A technician has ~8 hours per day. Don't over-allocate.

Return ONLY valid JSON — no markdown, no code fences, no explanation. Return an array of objects:
[
  {
    "project": "exact project name from the report",
    "company": "company name",
    "ticket_id": 123456,
    "next_step": "the planned next step text",
    "estimated_hours": 2.0
  }
]

Here is the report data:
`

// EstimateTaskDurations asks the AI to estimate hours for each project's next step.
func EstimateTaskDurations(cfg *config.Config, reportBody string, grouped map[string][]connectwise.TimeEntry) ([]TaskEstimate, error) {
	// Build context with ticket IDs
	var context strings.Builder
	context.WriteString(reportBody)
	context.WriteString("\n\nTicket reference data:\n")
	for projectName, entries := range grouped {
		for _, e := range entries {
			if e.Ticket != nil && e.Ticket.ID > 0 {
				company := "Unknown"
				if e.Company != nil {
					company = e.Company.Name
				}
				context.WriteString(fmt.Sprintf("- Project: %s | Company: %s | Ticket: #%d\n", projectName, company, e.Ticket.ID))
				break // One ticket per project is enough
			}
		}
	}

	fullPrompt := estimatePrompt + context.String()

	// Use whatever AI provider is configured
	summarizer, err := NewSummarizer(cfg, "")
	if err != nil {
		return nil, fmt.Errorf("creating AI client for estimation: %w", err)
	}

	// We need raw text back, so use the underlying provider
	var responseText string

	switch s := summarizer.(type) {
	case *GeminiSummarizer:
		body := map[string]any{
			"contents": []map[string]any{
				{"parts": []map[string]string{{"text": fullPrompt}}},
			},
		}
		jsonBody, _ := json.Marshal(body)
		url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", s.model, s.apiKey)
		resp, err := doPost(url, jsonBody, nil)
		if err != nil {
			return nil, fmt.Errorf("gemini estimation: %w", err)
		}
		var result struct {
			Candidates []struct {
				Content struct {
					Parts []struct {
						Text string `json:"text"`
					} `json:"parts"`
				} `json:"content"`
			} `json:"candidates"`
		}
		json.Unmarshal(resp, &result)
		if len(result.Candidates) > 0 && len(result.Candidates[0].Content.Parts) > 0 {
			responseText = result.Candidates[0].Content.Parts[0].Text
		}

	case *OpenAISummarizer:
		body := map[string]any{
			"model": s.model,
			"messages": []map[string]string{
				{"role": "system", "content": "You are a scheduling assistant. Return only valid JSON."},
				{"role": "user", "content": fullPrompt},
			},
			"temperature": 0.3,
		}
		jsonBody, _ := json.Marshal(body)
		headers := map[string]string{"Authorization": "Bearer " + s.apiKey}
		resp, err := doPost("https://api.openai.com/v1/chat/completions", jsonBody, headers)
		if err != nil {
			return nil, fmt.Errorf("openai estimation: %w", err)
		}
		var result struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		json.Unmarshal(resp, &result)
		if len(result.Choices) > 0 {
			responseText = result.Choices[0].Message.Content
		}

	case *AnthropicSummarizer:
		body := map[string]any{
			"model":      s.model,
			"max_tokens": 2048,
			"messages": []map[string]string{
				{"role": "user", "content": fullPrompt},
			},
		}
		jsonBody, _ := json.Marshal(body)
		headers := map[string]string{
			"x-api-key":         s.apiKey,
			"anthropic-version": "2023-06-01",
		}
		resp, err := doPost("https://api.anthropic.com/v1/messages", jsonBody, headers)
		if err != nil {
			return nil, fmt.Errorf("anthropic estimation: %w", err)
		}
		var result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		}
		json.Unmarshal(resp, &result)
		if len(result.Content) > 0 {
			responseText = result.Content[0].Text
		}
	}

	if responseText == "" {
		return nil, fmt.Errorf("empty response from AI for schedule estimation")
	}

	// Strip any markdown fencing the AI might add
	responseText = strings.TrimSpace(responseText)
	responseText = strings.TrimPrefix(responseText, "```json")
	responseText = strings.TrimPrefix(responseText, "```")
	responseText = strings.TrimSuffix(responseText, "```")
	responseText = strings.TrimSpace(responseText)

	var estimates []TaskEstimate
	if err := json.Unmarshal([]byte(responseText), &estimates); err != nil {
		return nil, fmt.Errorf("parsing AI estimates (got: %s): %w", responseText[:min(len(responseText), 200)], err)
	}

	return estimates, nil
}
