package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/weekly-report/internal/config"
	"github.com/weekly-report/internal/connectwise"
)

// ReportTemplate is the system prompt for AI summarization.
const ReportTemplate = `You are generating a weekly project progress report from timesheet entries.

For EACH project, you MUST follow this EXACT format structure. Use a numbered list (1., 2., 3., 4.) with nested bullet points (*):

{Company Name} | {Project Name} (#{ticket_numbers})

1. Project is On-Track: Yes
   * End Date: {date like "January 16" or "Ongoing"}
   * Notes: {brief context - ONLY include this bullet if there are relevant notes}
2. ConnectWise Calendar set for week(s) Ahead: Yes
3. Completed This Week:
   * {Summarize accomplishments as bullet points. If no work done, write "No updates; focus remained on other priorities."}
4. Planned for Next Week:
   * {ONE single, specific next step. See NEXT STEP RULES below. If work is complete, write "N/A (task completed)."}

CRITICAL FORMATTING RULES:
1. The project header line should include company name, project name, and ticket numbers - NO BOLD, NO ASTERISKS
2. Always use numbered list 1-4 for the main items
3. Use nested bullet points (*) for sub-items under each numbered item
4. End Date should be formatted as "January 16" not "2026-01-16"
5. Notes bullet is OPTIONAL - only include if there's meaningful context to add
6. ConnectWise Calendar is typically "Yes" unless it's a completed project
7. Group all time entries by project and summarize into cohesive bullet points
8. Use professional, concise language in past tense for "Completed This Week"
9. Separate each project section with a blank line
10. ABSOLUTELY NO MARKDOWN - no ** symbols, no __ symbols, no # symbols - this is plain text for Outlook email
11. Output ONLY plain text that can be directly pasted into an email

NEXT STEP RULES (for section 4 "Planned for Next Week"):
- Provide EXACTLY ONE next step per project. Do NOT list multiple bullet points. Techs should not feel overwhelmed.
- The next step must be the single most important, logical action that naturally follows from what was completed this week.
- Think carefully about what phase the project is in:
  * If work just started (setup, discovery, initial config): suggest the next build/implementation step.
  * If actively building (mid-project): suggest the next milestone or deliverable, not a repeat of what was already done.
  * If nearing completion (testing, final config, handoff): suggest validation, documentation, or client handoff.
  * If the project end date is approaching: prioritize the most critical remaining deliverable.
  * If only minor/routine work was logged: suggest continuing the main project objective rather than restating the minor task.
- Be specific and actionable. Bad: "Continue working on the project." Good: "Deploy updated bot to production Teams environment and confirm proactive messaging with Rewst."
- Never suggest generic filler like "follow up" or "continue progress" -- every next step should be something concrete the tech can act on.

EXAMPLE OUTPUT (follow this EXACT structure with NO markdown formatting):
Infinity Network Solutions | Automation: Teams Bot (InfiniBot) (#209066 / #209067)

1. Project is On-Track: Yes
   * End Date: January 16
   * Notes: Progress on building and deploying internal Teams bot for automation integration.
2. ConnectWise Calendar set for week(s) Ahead: Yes
3. Completed This Week:
   * Built Azure bot resources, validated manifest, resolved identity mismatch, and published bot for testing.
4. Planned for Next Week:
   * Deploy bot to production Teams environment and validate proactive messaging triggers from Rewst.
`

// Summarizer is the interface for AI text summarization.
type Summarizer interface {
	SummarizeEntries(grouped map[string][]connectwise.TimeEntry, reportDate string) (string, error)
	ProviderName() string
}

// NewSummarizer creates the appropriate summarizer based on config.
func NewSummarizer(cfg *config.Config, preferredProvider string) (Summarizer, error) {
	type providerInfo struct {
		key     string
		factory func(cfg *config.Config) Summarizer
	}

	providers := map[string]providerInfo{
		"gemini":    {cfg.GoogleAPIKey, func(c *config.Config) Summarizer { return NewGeminiSummarizer(c) }},
		"openai":    {cfg.OpenAIAPIKey, func(c *config.Config) Summarizer { return NewOpenAISummarizer(c) }},
		"anthropic": {cfg.AnthropicAPIKey, func(c *config.Config) Summarizer { return NewAnthropicSummarizer(c) }},
	}

	// Check preferred provider first
	if preferredProvider != "" {
		p := strings.ToLower(preferredProvider)
		if info, ok := providers[p]; ok && isValidKey(info.key) {
			return info.factory(cfg), nil
		}
	}

	// Check configured AI_PROVIDER env var
	if cfg.AIProvider != "" {
		p := strings.ToLower(cfg.AIProvider)
		if info, ok := providers[p]; ok && isValidKey(info.key) {
			return info.factory(cfg), nil
		}
	}

	// Auto-detect: use whichever key is actually populated (gemini > openai > anthropic)
	for _, name := range []string{"gemini", "openai", "anthropic"} {
		info := providers[name]
		if isValidKey(info.key) {
			return info.factory(cfg), nil
		}
	}

	return nil, fmt.Errorf("no AI API key configured; set GOOGLE_API_KEY, OPENAI_API_KEY, or ANTHROPIC_API_KEY")
}

// isValidKey returns true if the key looks like a real API key (not empty or a placeholder).
func isValidKey(key string) bool {
	if key == "" {
		return false
	}
	lower := strings.ToLower(key)
	return !strings.HasPrefix(lower, "your_") && lower != "changeme" && lower != "placeholder"
}

// buildPrompt constructs the full prompt from grouped entries.
func buildPrompt(grouped map[string][]connectwise.TimeEntry, reportDate string) string {
	var b strings.Builder
	b.WriteString(ReportTemplate)
	b.WriteString("\n\nHere are the timesheet entries to summarize:\n\n")

	for projectName, entries := range grouped {
		b.WriteString(fmt.Sprintf("--- PROJECT: %s ---\n", projectName))
		for _, e := range entries {
			company := "Unknown Company"
			if e.Company != nil && e.Company.Name != "" {
				company = e.Company.Name
			} else if e.TicketDetails != nil && e.TicketDetails.Company != nil {
				company = e.TicketDetails.Company.Name
			} else if e.ProjectDetails != nil && e.ProjectDetails.Company != nil {
				company = e.ProjectDetails.Company.Name
			}

			ticketID := ""
			if e.Ticket != nil && e.Ticket.ID > 0 {
				ticketID = fmt.Sprintf("#%d", e.Ticket.ID)
			} else if e.TicketDetails != nil && e.TicketDetails.ID > 0 {
				ticketID = fmt.Sprintf("#%d", e.TicketDetails.ID)
			}

			endDate := "Ongoing"
			if e.ProjectDetails != nil && e.ProjectDetails.EstimatedEnd != "" {
				endDate = e.ProjectDetails.EstimatedEnd
			}

			notes := e.Notes
			if notes == "" {
				notes = "No notes provided"
			}

			b.WriteString(fmt.Sprintf("  Company: %s\n", company))
			b.WriteString(fmt.Sprintf("  Date: %s\n", e.DateEntered))
			b.WriteString(fmt.Sprintf("  Hours: %.1f\n", e.ActualHours))
			b.WriteString(fmt.Sprintf("  Ticket: %s\n", ticketID))
			b.WriteString(fmt.Sprintf("  End Date: %s\n", endDate))
			b.WriteString(fmt.Sprintf("  Notes: %s\n\n", notes))
		}
		b.WriteString("\n")
	}

	b.WriteString(fmt.Sprintf("\nPlease generate the weekly report for the week ending %s.", reportDate))
	return b.String()
}

// ── Gemini ──────────────────────────────────────────────────────────────

const defaultGeminiModel = "gemini-pro-latest"

type GeminiSummarizer struct {
	apiKey string
	model  string
}

func NewGeminiSummarizer(cfg *config.Config) *GeminiSummarizer {
	model := cfg.AIModel
	if model == "" {
		model = defaultGeminiModel
	}
	return &GeminiSummarizer{apiKey: cfg.GoogleAPIKey, model: model}
}

func (g *GeminiSummarizer) ProviderName() string {
	return fmt.Sprintf("Google Gemini (%s)", g.model)
}

func (g *GeminiSummarizer) SummarizeEntries(grouped map[string][]connectwise.TimeEntry, reportDate string) (string, error) {
	prompt := buildPrompt(grouped, reportDate)

	body := map[string]any{
		"contents": []map[string]any{
			{"parts": []map[string]string{{"text": prompt}}},
		},
	}
	jsonBody, _ := json.Marshal(body)

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", g.model, g.apiKey)
	resp, err := doPost(url, jsonBody, nil)
	if err != nil {
		return "", fmt.Errorf("gemini API: %w", err)
	}

	// Parse response
	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return "", fmt.Errorf("parsing gemini response: %w", err)
	}
	if len(result.Candidates) > 0 && len(result.Candidates[0].Content.Parts) > 0 {
		return result.Candidates[0].Content.Parts[0].Text, nil
	}
	return "", fmt.Errorf("empty response from Gemini")
}

// ── OpenAI ──────────────────────────────────────────────────────────────

const defaultOpenAIModel = "gpt-5.1"

type OpenAISummarizer struct {
	apiKey string
	model  string
}

func NewOpenAISummarizer(cfg *config.Config) *OpenAISummarizer {
	model := cfg.AIModel
	if model == "" {
		model = defaultOpenAIModel
	}
	return &OpenAISummarizer{apiKey: cfg.OpenAIAPIKey, model: model}
}

func (o *OpenAISummarizer) ProviderName() string {
	return fmt.Sprintf("OpenAI (%s)", o.model)
}

func (o *OpenAISummarizer) SummarizeEntries(grouped map[string][]connectwise.TimeEntry, reportDate string) (string, error) {
	prompt := buildPrompt(grouped, reportDate)

	body := map[string]any{
		"model": o.model,
		"messages": []map[string]string{
			{"role": "system", "content": "You are a professional report generator."},
			{"role": "user", "content": prompt},
		},
		"temperature": 0.7,
	}
	jsonBody, _ := json.Marshal(body)

	headers := map[string]string{
		"Authorization": "Bearer " + o.apiKey,
	}
	resp, err := doPost("https://api.openai.com/v1/chat/completions", jsonBody, headers)
	if err != nil {
		return "", fmt.Errorf("openai API: %w", err)
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return "", fmt.Errorf("parsing openai response: %w", err)
	}
	if len(result.Choices) > 0 {
		return result.Choices[0].Message.Content, nil
	}
	return "", fmt.Errorf("empty response from OpenAI")
}

// ── Anthropic ───────────────────────────────────────────────────────────

const defaultAnthropicModel = "claude-sonnet-4-20250514"

type AnthropicSummarizer struct {
	apiKey string
	model  string
}

func NewAnthropicSummarizer(cfg *config.Config) *AnthropicSummarizer {
	model := cfg.AIModel
	if model == "" {
		model = defaultAnthropicModel
	}
	return &AnthropicSummarizer{apiKey: cfg.AnthropicAPIKey, model: model}
}

func (a *AnthropicSummarizer) ProviderName() string {
	return fmt.Sprintf("Anthropic (%s)", a.model)
}

func (a *AnthropicSummarizer) SummarizeEntries(grouped map[string][]connectwise.TimeEntry, reportDate string) (string, error) {
	prompt := buildPrompt(grouped, reportDate)

	body := map[string]any{
		"model":      a.model,
		"max_tokens": 4096,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}
	jsonBody, _ := json.Marshal(body)

	headers := map[string]string{
		"x-api-key":         a.apiKey,
		"anthropic-version": "2023-06-01",
	}
	resp, err := doPost("https://api.anthropic.com/v1/messages", jsonBody, headers)
	if err != nil {
		return "", fmt.Errorf("anthropic API: %w", err)
	}

	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return "", fmt.Errorf("parsing anthropic response: %w", err)
	}
	if len(result.Content) > 0 {
		return result.Content[0].Text, nil
	}
	return "", fmt.Errorf("empty response from Anthropic")
}

// ── Helpers ─────────────────────────────────────────────────────────────

func doPost(url string, body []byte, extraHeaders map[string]string) ([]byte, error) {
	client := &http.Client{Timeout: 120 * time.Second}

	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody[:min(len(respBody), 500)]))
	}

	return respBody, nil
}
