package report

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/weekly-report/internal/connectwise"
)

// ExcludePatterns are project name patterns to filter out (case-insensitive).
var ExcludePatterns = []string{
	"meeting",
	"internal",
	"z - internal",
	"admin",
	"pto",
	"vacation",
	"holiday",
	"sick",
}

// Generator handles report grouping, formatting, and file output.
type Generator struct {
	MemberInitials string
	OutputDir      string
}

// NewGenerator creates a Generator with the given initials.
func NewGenerator(initials string) *Generator {
	dir, _ := os.Getwd()
	outDir := filepath.Join(dir, "reports")
	os.MkdirAll(outDir, 0755)
	return &Generator{
		MemberInitials: initials,
		OutputDir:      outDir,
	}
}

// GroupEntriesByProject groups time entries by project name, excluding filtered categories.
func (g *Generator) GroupEntriesByProject(entries []connectwise.TimeEntry) map[string][]connectwise.TimeEntry {
	grouped := make(map[string][]connectwise.TimeEntry)

	for _, e := range entries {
		name := getProjectName(e)
		if shouldExclude(name) {
			continue
		}
		grouped[name] = append(grouped[name], e)
	}

	return grouped
}

// GenerateEmailSubject formats the email subject line.
func (g *Generator) GenerateEmailSubject(reportDate time.Time) string {
	return fmt.Sprintf("%s | Weekly Project Progress Report | %s", g.MemberInitials, reportDate.Format("2006-01-02"))
}

// WriteReport writes the report to a text file and returns the filepath.
func (g *Generator) WriteReport(subject, body string, reportDate time.Time) (string, error) {
	dateStr := reportDate.Format("2006-01-02")
	filename := fmt.Sprintf("weekly_report_%s.txt", dateStr)
	fpath := filepath.Join(g.OutputDir, filename)

	sep := strings.Repeat("=", 60)
	dash := strings.Repeat("-", 60)

	content := fmt.Sprintf("%s\nWEEKLY PROJECT PROGRESS REPORT\n%s\n\nSubject: %s\n%s\n\nBody:\n\n%s\n\n%s\nEND OF REPORT\n%s\n",
		sep, sep, subject, dash, body, sep, sep)

	if err := os.WriteFile(fpath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("writing report: %w", err)
	}

	return fpath, nil
}

// GenerateHTMLEmail converts a plain-text report to simple HTML for email.
func GenerateHTMLEmail(subject, body string) string {
	htmlBody := strings.ReplaceAll(body, "\n", "<br>\n")

	// Bold standard section headers
	replacements := map[string]string{
		"1. Project is On-Track:":   "<b>1. Project is On-Track:</b>",
		"2. ConnectWise Calendar":   "<b>2. ConnectWise Calendar</b>",
		"3. Completed This Week:":   "<b>3. Completed This Week:</b>",
		"4. Planned for Next Week:": "<b>4. Planned for Next Week:</b>",
	}
	for old, new := range replacements {
		htmlBody = strings.ReplaceAll(htmlBody, old, new)
	}

	// Bold project headers (lines containing | and ending with ))
	var lines []string
	for _, line := range strings.Split(htmlBody, "<br>\n") {
		if strings.Contains(line, "|") && (strings.Contains(line, "(") || strings.HasSuffix(strings.TrimSpace(line), ")")) {
			if !strings.HasPrefix(strings.TrimSpace(line), "<b>") {
				line = "<b>" + line + "</b>"
			}
		}
		lines = append(lines, line)
	}
	htmlBody = strings.Join(lines, "<br>\n")

	return fmt.Sprintf(`<html>
<head>
    <style>
        body { font-family: Aptos, Calibri, Helvetica, sans-serif; font-size: 11pt; line-height: 1.5; }
        b { font-weight: bold; }
    </style>
</head>
<body>
    %s
</body>
</html>`, htmlBody)
}

func getProjectName(e connectwise.TimeEntry) string {
	if e.ProjectDetails != nil && e.ProjectDetails.Name != "" {
		return e.ProjectDetails.Name
	}
	if e.Project != nil && e.Project.Name != "" {
		return e.Project.Name
	}
	if e.TicketDetails != nil && e.TicketDetails.Summary != "" {
		return e.TicketDetails.Summary
	}
	if e.Ticket != nil && e.Ticket.Name != "" {
		return e.Ticket.Name
	}
	if e.WorkType != nil && e.WorkType.Name != "" {
		return "General: " + e.WorkType.Name
	}
	return "Uncategorized Work"
}

func shouldExclude(name string) bool {
	lower := strings.ToLower(name)
	for _, pattern := range ExcludePatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}
