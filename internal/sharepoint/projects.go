package sharepoint

import (
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"
)

// Project represents a project from the SharePoint roadmap.
type Project struct {
	Name      string `json:"name"`
	Status    string `json:"status,omitempty"`
	Owner     string `json:"owner,omitempty"`
	StartDate string `json:"start_date,omitempty"`
	EndDate   string `json:"end_date,omitempty"`
}

// ComparisonResult holds the results of comparing roadmap with timesheet.
type ComparisonResult struct {
	InBoth        []MatchedProject `json:"in_both"`
	RoadmapOnly   []Project        `json:"roadmap_only"`
	TimesheetOnly []string         `json:"timesheet_only"`
}

// MatchedProject is a project found in both roadmap and timesheet.
type MatchedProject struct {
	Roadmap   string `json:"roadmap"`
	Timesheet string `json:"timesheet"`
}

// ParseRoadmap reads the Excel file and extracts project information.
func ParseRoadmap(excelPath string) ([]Project, error) {
	f, err := excelize.OpenFile(excelPath)
	if err != nil {
		return nil, fmt.Errorf("opening Excel: %w", err)
	}
	defer f.Close()

	// Find the right sheet
	sheetName := ""
	for _, name := range []string{"Projects", "Roadmap", "Active", "2025", "Sheet1"} {
		if idx, _ := f.GetSheetIndex(name); idx >= 0 {
			sheetName = name
			break
		}
	}
	if sheetName == "" {
		sheetName = f.GetSheetName(0)
	}

	rows, err := f.GetRows(sheetName)
	if err != nil {
		return nil, fmt.Errorf("reading sheet %s: %w", sheetName, err)
	}

	if len(rows) < 2 {
		return nil, fmt.Errorf("sheet has no data rows")
	}

	// Parse header row
	headers := make(map[string]int)
	for i, cell := range rows[0] {
		headers[strings.ToLower(strings.TrimSpace(cell))] = i
	}

	nameCol := findCol(headers, []string{"project", "project name", "name", "title"})
	statusCol := findCol(headers, []string{"status", "project status", "state"})
	ownerCol := findCol(headers, []string{"owner", "assigned to", "assigned", "resource", "technician"})
	startCol := findCol(headers, []string{"start", "start date", "begin"})
	endCol := findCol(headers, []string{"end", "end date", "due", "due date", "target"})

	if nameCol < 0 {
		return nil, fmt.Errorf("could not find project name column in Excel")
	}

	var projects []Project
	for _, row := range rows[1:] {
		if nameCol >= len(row) || strings.TrimSpace(row[nameCol]) == "" {
			continue
		}

		p := Project{
			Name: strings.TrimSpace(row[nameCol]),
		}

		if statusCol >= 0 && statusCol < len(row) {
			p.Status = strings.TrimSpace(row[statusCol])
		}
		if ownerCol >= 0 && ownerCol < len(row) {
			p.Owner = strings.TrimSpace(row[ownerCol])
		}
		if startCol >= 0 && startCol < len(row) {
			p.StartDate = strings.TrimSpace(row[startCol])
		}
		if endCol >= 0 && endCol < len(row) {
			p.EndDate = strings.TrimSpace(row[endCol])
		}

		projects = append(projects, p)
	}

	return projects, nil
}

// GetActiveProjects filters projects to active ones, optionally by owner.
func GetActiveProjects(projects []Project, ownerFilter string) []Project {
	var active []Project

	for _, p := range projects {
		status := strings.ToLower(p.Status)

		// Skip completed/closed
		if containsAny(status, []string{"complete", "closed", "done", "cancelled", "canceled"}) {
			continue
		}

		// Check if active (active, in progress, open, current, or empty status)
		isActive := status == "" || containsAny(status, []string{"active", "progress", "open", "current"})
		if !isActive {
			continue
		}

		// Filter by owner
		if ownerFilter != "" {
			if !strings.Contains(strings.ToLower(p.Owner), strings.ToLower(ownerFilter)) {
				continue
			}
		}

		active = append(active, p)
	}

	return active
}

// CompareWithTimesheet compares roadmap projects with timesheet project names.
func CompareWithTimesheet(projects []Project, timesheetProjects []string, ownerFilter string) ComparisonResult {
	roadmapProjects := GetActiveProjects(projects, ownerFilter)

	roadmapMap := make(map[string]Project)
	for _, p := range roadmapProjects {
		roadmapMap[strings.ToLower(p.Name)] = p
	}

	tsMap := make(map[string]string)
	for _, name := range timesheetProjects {
		tsMap[strings.ToLower(name)] = name
	}

	var result ComparisonResult

	// Check roadmap projects
	for nameLower, project := range roadmapMap {
		matched := false
		for tsLower, tsName := range tsMap {
			if strings.Contains(nameLower, tsLower) || strings.Contains(tsLower, nameLower) {
				result.InBoth = append(result.InBoth, MatchedProject{
					Roadmap:   project.Name,
					Timesheet: tsName,
				})
				matched = true
				break
			}
		}
		if !matched {
			result.RoadmapOnly = append(result.RoadmapOnly, project)
		}
	}

	// Check timesheet projects not in roadmap
	for tsLower, tsName := range tsMap {
		matched := false
		for nameLower := range roadmapMap {
			if strings.Contains(nameLower, tsLower) || strings.Contains(tsLower, nameLower) {
				matched = true
				break
			}
		}
		if !matched {
			result.TimesheetOnly = append(result.TimesheetOnly, tsName)
		}
	}

	return result
}

// FormatComparisonSummary formats comparison results as a readable string.
func FormatComparisonSummary(result ComparisonResult) string {
	var lines []string

	if len(result.RoadmapOnly) > 0 {
		lines = append(lines, "\n⚠️  Projects in Roadmap but MISSING from this week's report:")
		for _, p := range result.RoadmapOnly {
			endDate := p.EndDate
			if endDate == "" {
				endDate = "N/A"
			}
			lines = append(lines, fmt.Sprintf("   - %s (Due: %s)", p.Name, endDate))
		}
	}

	if len(result.TimesheetOnly) > 0 {
		lines = append(lines, "\n📝 Projects with time entries but NOT in Roadmap:")
		for _, name := range result.TimesheetOnly {
			lines = append(lines, fmt.Sprintf("   - %s", name))
		}
	}

	if len(lines) == 0 {
		lines = append(lines, "\n✅ All roadmap projects are accounted for in this week's report.")
	}

	return strings.Join(lines, "\n")
}

func findCol(headers map[string]int, options []string) int {
	for _, opt := range options {
		if idx, ok := headers[opt]; ok {
			return idx
		}
	}
	return -1
}

func containsAny(s string, substrs []string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
