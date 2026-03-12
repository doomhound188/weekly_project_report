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
		// Skip entries that don't belong to a project
		if (e.Project == nil || e.Project.ID == 0) && e.ProjectDetails == nil {
			continue
		}
		name := getProjectName(e)
		if shouldExclude(name) {
			continue
		}
		grouped[name] = append(grouped[name], e)
	}

	return grouped
}

// CollectNonProjectEntries builds a summary of non-project work (tickets, general tasks)
// to provide as context to the AI for citing "other priorities".
func (g *Generator) CollectNonProjectEntries(entries []connectwise.TimeEntry) string {
	type taskSummary struct {
		name  string
		hours float64
	}
	tasks := make(map[string]*taskSummary)

	for _, e := range entries {
		// Only collect entries that DON'T belong to a project
		if (e.Project != nil && e.Project.ID > 0) || e.ProjectDetails != nil {
			continue
		}

		name := ""
		if e.TicketDetails != nil && e.TicketDetails.Summary != "" {
			name = e.TicketDetails.Summary
		} else if e.Ticket != nil && e.Ticket.Name != "" {
			name = e.Ticket.Name
		} else if e.WorkType != nil && e.WorkType.Name != "" {
			name = e.WorkType.Name
		} else {
			name = "General work"
		}

		if shouldExclude(name) {
			continue
		}

		if _, ok := tasks[name]; !ok {
			tasks[name] = &taskSummary{name: name}
		}
		tasks[name].hours += e.ActualHours
	}

	if len(tasks) == 0 {
		return ""
	}

	var lines []string
	for _, t := range tasks {
		lines = append(lines, fmt.Sprintf("- %s (%.1f hrs)", t.name, t.hours))
	}
	return "Non-project work this week:\n" + strings.Join(lines, "\n")
}

// MergeAssignedProjects ensures every assigned project appears in the grouped map,
// adding an empty entry slice for projects with no time entries.
func (g *Generator) MergeAssignedProjects(grouped map[string][]connectwise.TimeEntry, assignedProjects []connectwise.ProjectDetails) {
	for _, p := range assignedProjects {
		if p.Name == "" || shouldExclude(p.Name) {
			continue
		}
		// Check if project is already in grouped (by name match)
		found := false
		for name := range grouped {
			if name == p.Name {
				found = true
				break
			}
		}
		if !found {
			grouped[p.Name] = nil // empty slice = no time entries
		}
	}
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
	return "Unknown Project"
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
