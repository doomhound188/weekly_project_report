package schedule

import (
	"testing"
	"time"

	"github.com/weekly-report/internal/ai"
	"github.com/weekly-report/internal/connectwise"
)

// ── isIgnorableActivity tests ───────────────────────────────────────────

func TestIsIgnorableActivity(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"lunch entry", "Lunch Break", true},
		{"meeting entry", "Team Meeting", true},
		{"meetings plural", "Weekly Meetings", true},
		{"no company entry", "No company - Internal", true},
		{"normal project", "Server Migration", false},
		{"empty string", "", false},
		{"case insensitive lunch", "LUNCH", true},
		{"case insensitive meeting", "daily meeting standup", true},
		{"case insensitive no company", "NO COMPANY", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isIgnorableActivity(tt.input)
			if result != tt.expected {
				t.Errorf("isIgnorableActivity(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsIgnorableEntry(t *testing.T) {
	tests := []struct {
		name     string
		entry    connectwise.ScheduleEntry
		expected bool
	}{
		{
			"ignorable by name",
			connectwise.ScheduleEntry{Name: "Lunch Break"},
			true,
		},
		{
			"ignorable by company name",
			connectwise.ScheduleEntry{
				Name:    "Some Task",
				Company: &connectwise.Ref{Name: "No Company"},
			},
			true,
		},
		{
			"not ignorable",
			connectwise.ScheduleEntry{
				Name:    "Deploy Server",
				Company: &connectwise.Ref{Name: "Acme Corp"},
			},
			false,
		},
		{
			"nil company not ignorable",
			connectwise.ScheduleEntry{Name: "Deploy Server"},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isIgnorableEntry(tt.entry)
			if result != tt.expected {
				t.Errorf("isIgnorableEntry(%q) = %v, want %v", tt.entry.Name, result, tt.expected)
			}
		})
	}
}

// ── Task splitting tests ────────────────────────────────────────────────

func TestPlanWeek_TaskSplitting(t *testing.T) {
	loc := time.UTC

	// Create a calendar with 8-hour days
	calendar := &connectwise.MemberCalendar{
		MondayStart:    "09:00",
		MondayEnd:      "17:00",
		TuesdayStart:   "09:00",
		TuesdayEnd:     "17:00",
		WednesdayStart: "09:00",
		WednesdayEnd:   "17:00",
		ThursdayStart:  "09:00",
		ThursdayEnd:    "17:00",
		FridayStart:    "09:00",
		FridayEnd:      "17:00",
	}

	// A task that takes 10 hours — more than a single 8-hour day
	estimates := []ai.TaskEstimate{
		{
			Project:        "Big Migration",
			Company:        "Acme Corp",
			TicketID:       12345,
			NextStep:       "Run migration scripts",
			EstimatedHours: 10.0,
		},
	}

	plan := PlanWeek(estimates, nil, calendar, nil, loc)

	// Should produce at least 2 blocks (split across days)
	if len(plan.Blocks) < 2 {
		t.Errorf("Expected at least 2 blocks for 10-hour task on 8-hour days, got %d", len(plan.Blocks))
	}

	// Total hours across all blocks should equal 10
	var totalHours float64
	for _, b := range plan.Blocks {
		totalHours += b.Hours
	}
	if totalHours < 9.9 || totalHours > 10.1 {
		t.Errorf("Expected total scheduled hours ~10.0, got %.1f", totalHours)
	}

	// Nothing should be unscheduled (5 days × 8 hours = 40 hours available)
	if len(plan.Unscheduled) > 0 {
		t.Errorf("Expected no unscheduled tasks, got %d", len(plan.Unscheduled))
	}
}

func TestPlanWeek_TaskFitsInOneDay(t *testing.T) {
	loc := time.UTC

	calendar := &connectwise.MemberCalendar{
		MondayStart:    "09:00",
		MondayEnd:      "17:00",
		TuesdayStart:   "09:00",
		TuesdayEnd:     "17:00",
		WednesdayStart: "09:00",
		WednesdayEnd:   "17:00",
		ThursdayStart:  "09:00",
		ThursdayEnd:    "17:00",
		FridayStart:    "09:00",
		FridayEnd:      "17:00",
	}

	estimates := []ai.TaskEstimate{
		{
			Project:        "Small Fix",
			Company:        "Acme Corp",
			TicketID:       99999,
			NextStep:       "Fix DNS",
			EstimatedHours: 4.0,
		},
	}

	plan := PlanWeek(estimates, nil, calendar, nil, loc)

	// Should be exactly 1 block since 4 hours fits in an 8-hour day
	if len(plan.Blocks) != 1 {
		t.Errorf("Expected 1 block for 4-hour task, got %d", len(plan.Blocks))
	}

	if len(plan.Blocks) > 0 && plan.Blocks[0].Hours != 4.0 {
		t.Errorf("Expected block hours = 4.0, got %.1f", plan.Blocks[0].Hours)
	}
}

func TestPlanWeek_IgnoresLunchAndNoCompany(t *testing.T) {
	loc := time.UTC

	calendar := &connectwise.MemberCalendar{
		MondayStart:  "09:00",
		MondayEnd:    "17:00",
		TuesdayStart: "09:00",
		TuesdayEnd:   "17:00",
	}

	// Next Monday at 9am - we need to compute the actual next monday
	now := time.Now().In(loc)
	daysUntilMonday := (8 - int(now.Weekday())) % 7
	if daysUntilMonday == 0 {
		daysUntilMonday = 7
	}
	nextMonday := time.Date(now.Year(), now.Month(), now.Day()+daysUntilMonday, 0, 0, 0, 0, loc)

	// Add a lunch entry and a "No company" entry that should be ignored
	existing := []connectwise.ScheduleEntry{
		{
			Name:      "Lunch Break",
			DateStart: nextMonday.Add(12 * time.Hour).Format(time.RFC3339), // 12pm
			DateEnd:   nextMonday.Add(13 * time.Hour).Format(time.RFC3339), // 1pm
		},
		{
			Name:      "Internal call",
			Company:   &connectwise.Ref{Name: "No Company"},
			DateStart: nextMonday.Add(13 * time.Hour).Format(time.RFC3339), // 1pm
			DateEnd:   nextMonday.Add(14 * time.Hour).Format(time.RFC3339), // 2pm
		},
	}

	estimates := []ai.TaskEstimate{
		{
			Project:        "Deploy",
			EstimatedHours: 8.0,
		},
	}

	plan := PlanWeek(estimates, existing, calendar, nil, loc)

	// With lunch and no company ignored, the full 8 hours should be available on Monday
	// The task should fit entirely on Monday
	if len(plan.Blocks) != 1 {
		t.Errorf("Expected 1 block (lunch+no company ignored), got %d", len(plan.Blocks))
	}

	if len(plan.Unscheduled) > 0 {
		t.Errorf("Expected no unscheduled tasks, got %d", len(plan.Unscheduled))
	}
}

func TestPlanWeek_BalancedDistribution(t *testing.T) {
	loc := time.UTC

	calendar := &connectwise.MemberCalendar{
		MondayStart:    "09:00",
		MondayEnd:      "17:00",
		TuesdayStart:   "09:00",
		TuesdayEnd:     "17:00",
		WednesdayStart: "09:00",
		WednesdayEnd:   "17:00",
		ThursdayStart:  "09:00",
		ThursdayEnd:    "17:00",
		FridayStart:    "09:00",
		FridayEnd:      "17:00",
	}

	// 5 tasks of 4 hours each = 20 hrs total, 40 hrs available
	// Each task should land on a different day
	estimates := []ai.TaskEstimate{
		{Project: "Project A", EstimatedHours: 4.0},
		{Project: "Project B", EstimatedHours: 4.0},
		{Project: "Project C", EstimatedHours: 4.0},
		{Project: "Project D", EstimatedHours: 4.0},
		{Project: "Project E", EstimatedHours: 4.0},
	}

	plan := PlanWeek(estimates, nil, calendar, nil, loc)

	if len(plan.Blocks) != 5 {
		t.Errorf("Expected 5 blocks, got %d", len(plan.Blocks))
	}

	// Tasks should be spread across at least 4 different days
	daysSeen := make(map[string]bool)
	for _, b := range plan.Blocks {
		daysSeen[b.Day] = true
	}
	if len(daysSeen) < 4 {
		t.Errorf("Expected tasks on at least 4 days, got %d: %v", len(daysSeen), daysSeen)
	}

	if len(plan.Unscheduled) > 0 {
		t.Errorf("Expected no unscheduled tasks, got %d", len(plan.Unscheduled))
	}
}
