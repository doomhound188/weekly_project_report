package schedule

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/weekly-report/internal/ai"
	"github.com/weekly-report/internal/connectwise"
)

// ProposedBlock represents a single scheduled time block.
type ProposedBlock struct {
	Project   string    `json:"project"`
	Company   string    `json:"company"`
	TicketID  int       `json:"ticket_id"`
	NextStep  string    `json:"next_step"`
	DateStart time.Time `json:"date_start"`
	DateEnd   time.Time `json:"date_end"`
	Hours     float64   `json:"hours"`
	Day       string    `json:"day"`
	TimeRange string    `json:"time_range"`
}

// WeekPlan holds the full proposed schedule for a week.
type WeekPlan struct {
	WeekStarting string            `json:"week_starting"`
	Blocks       []ProposedBlock   `json:"blocks"`
	Unscheduled  []ai.TaskEstimate `json:"unscheduled,omitempty"`
	TotalHours   float64           `json:"total_hours"`
	FreeHours    float64           `json:"free_hours"`
}

// timeSlot represents a free window in a day.
type timeSlot struct {
	start time.Time
	end   time.Time
}

// PlanWeek takes task estimates and existing schedule, then allocates blocks into free time.
func PlanWeek(
	estimates []ai.TaskEstimate,
	existing []connectwise.ScheduleEntry,
	calendar *connectwise.MemberCalendar,
	holidays []connectwise.Holiday,
	loc *time.Location,
) *WeekPlan {
	if loc == nil {
		loc = time.Now().Location()
	}

	// Determine next Monday
	now := time.Now().In(loc)
	daysUntilMonday := (8 - int(now.Weekday())) % 7
	if daysUntilMonday == 0 {
		daysUntilMonday = 7
	}
	nextMonday := time.Date(now.Year(), now.Month(), now.Day()+daysUntilMonday, 0, 0, 0, 0, loc)

	// Sort estimates by hours descending (big rocks first)
	sorted := make([]ai.TaskEstimate, len(estimates))
	copy(sorted, estimates)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].EstimatedHours > sorted[j].EstimatedHours
	})

	// Build free slots for Mon-Fri, skipping holidays/vacation/sick
	freeSlots := buildFreeSlots(nextMonday, calendar, existing, holidays, loc)

	plan := &WeekPlan{
		WeekStarting: nextMonday.Format("2006-01-02"),
	}

	var totalScheduled float64

	for _, est := range sorted {
		durationMins := int(est.EstimatedHours * 60)
		placed := false

		for dayIdx := range freeSlots {
			for slotIdx, slot := range freeSlots[dayIdx] {
				slotMins := int(slot.end.Sub(slot.start).Minutes())

				if slotMins >= durationMins {
					// Place block at the start of this slot
					blockEnd := slot.start.Add(time.Duration(durationMins) * time.Minute)

					block := ProposedBlock{
						Project:   est.Project,
						Company:   est.Company,
						TicketID:  est.TicketID,
						NextStep:  est.NextStep,
						DateStart: slot.start,
						DateEnd:   blockEnd,
						Hours:     est.EstimatedHours,
						Day:       slot.start.Format("Monday"),
						TimeRange: fmt.Sprintf("%s - %s", slot.start.Format("3:04 PM"), blockEnd.Format("3:04 PM")),
					}
					plan.Blocks = append(plan.Blocks, block)
					totalScheduled += est.EstimatedHours

					// Shrink the slot
					freeSlots[dayIdx][slotIdx] = timeSlot{
						start: blockEnd,
						end:   slot.end,
					}

					placed = true
					break
				}
			}
			if placed {
				break
			}
		}

		if !placed {
			plan.Unscheduled = append(plan.Unscheduled, est)
		}
	}

	// Sort blocks by start time
	sort.Slice(plan.Blocks, func(i, j int) bool {
		return plan.Blocks[i].DateStart.Before(plan.Blocks[j].DateStart)
	})

	// Calculate totals
	plan.TotalHours = totalScheduled
	var totalFree float64
	for _, daySlots := range freeSlots {
		for _, slot := range daySlots {
			totalFree += slot.end.Sub(slot.start).Hours()
		}
	}
	plan.FreeHours = totalFree

	return plan
}

// buildFreeSlots creates a slice of free time slots for each workday,
// skipping holidays, vacation days, and sick days entirely.
func buildFreeSlots(
	monday time.Time,
	calendar *connectwise.MemberCalendar,
	existing []connectwise.ScheduleEntry,
	holidays []connectwise.Holiday,
	loc *time.Location,
) [][]timeSlot {
	freeSlots := make([][]timeSlot, 5) // Mon-Fri

	// Build a set of blocked dates from holidays
	blockedDates := make(map[string]bool)
	for _, h := range holidays {
		// Parse the holiday date and add to blocked set
		for _, layout := range []string{"2006-01-02T15:04:05Z", "2006-01-02", time.RFC3339} {
			if t, err := time.Parse(layout, h.Date); err == nil {
				blockedDates[t.Format("2006-01-02")] = true
				break
			}
		}
	}

	// Also check existing entries for full-day vacation/sick/PTO
	for _, entry := range existing {
		if connectwise.IsVacationOrSickDay(entry) {
			entryStart := parseCWTime(entry.DateStart, loc)
			if !entryStart.IsZero() {
				blockedDates[entryStart.Format("2006-01-02")] = true
			}
		}
	}

	for dayOffset := 0; dayOffset < 5; dayOffset++ {
		day := monday.AddDate(0, 0, dayOffset)
		weekday := day.Weekday()

		// Skip holidays and vacation/sick days
		if blockedDates[day.Format("2006-01-02")] {
			continue
		}

		if !calendar.HasShift(weekday) {
			continue
		}

		startStr, endStr := calendar.ShiftForDay(weekday)
		shiftStart := parseTimeOnDay(day, startStr, loc)
		shiftEnd := parseTimeOnDay(day, endStr, loc)

		if shiftStart.IsZero() || shiftEnd.IsZero() || !shiftEnd.After(shiftStart) {
			continue
		}

		// Start with the full shift as one free slot
		dayFree := []timeSlot{{start: shiftStart, end: shiftEnd}}

		// Subtract existing schedule entries (skip ignorable activities)
		for _, entry := range existing {
			if isIgnorableActivity(entry.Name) {
				continue
			}

			entryStart := parseCWTime(entry.DateStart, loc)
			entryEnd := parseCWTime(entry.DateEnd, loc)

			if entryStart.IsZero() || entryEnd.IsZero() {
				continue
			}

			// Only consider entries on this day
			if entryStart.Day() != day.Day() || entryStart.Month() != day.Month() {
				continue
			}

			dayFree = subtractFromSlots(dayFree, entryStart, entryEnd)
		}

		// Remove tiny gaps (< 30 min)
		var filtered []timeSlot
		for _, slot := range dayFree {
			if slot.end.Sub(slot.start).Minutes() >= 30 {
				filtered = append(filtered, slot)
			}
		}

		freeSlots[dayOffset] = filtered
	}

	return freeSlots
}

// isIgnorableActivity returns true for schedule entries that should not block
// task scheduling (e.g. recurring lunch breaks, internal meetings).
func isIgnorableActivity(name string) bool {
	lower := strings.ToLower(name)
	return strings.Contains(lower, "lunch") || strings.Contains(lower, "meetings")
}

// subtractFromSlots removes a busy period from the free slots.
func subtractFromSlots(slots []timeSlot, busyStart, busyEnd time.Time) []timeSlot {
	var result []timeSlot

	for _, slot := range slots {
		// No overlap
		if busyEnd.Before(slot.start) || busyEnd.Equal(slot.start) ||
			busyStart.After(slot.end) || busyStart.Equal(slot.end) {
			result = append(result, slot)
			continue
		}

		// Before the busy block
		if slot.start.Before(busyStart) {
			result = append(result, timeSlot{start: slot.start, end: busyStart})
		}

		// After the busy block
		if slot.end.After(busyEnd) {
			result = append(result, timeSlot{start: busyEnd, end: slot.end})
		}
	}

	return result
}

// parseTimeOnDay combines a date with a HH:MM time string.
func parseTimeOnDay(day time.Time, timeStr string, loc *time.Location) time.Time {
	timeStr = strings.TrimSpace(timeStr)
	if timeStr == "" {
		return time.Time{}
	}

	parts := strings.Split(timeStr, ":")
	if len(parts) != 2 {
		return time.Time{}
	}

	var hour, min int
	fmt.Sscanf(parts[0], "%d", &hour)
	fmt.Sscanf(parts[1], "%d", &min)

	return time.Date(day.Year(), day.Month(), day.Day(), hour, min, 0, 0, loc)
}

// parseCWTime parses a ConnectWise datetime string.
func parseCWTime(s string, loc *time.Location) time.Time {
	if s == "" {
		return time.Time{}
	}

	// Try ISO 8601 formats
	for _, layout := range []string{
		time.RFC3339,
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05",
		"2006-01-02T15:04:05-07:00",
	} {
		if t, err := time.ParseInLocation(layout, s, loc); err == nil {
			return t.In(loc)
		}
	}

	return time.Time{}
}

// FormatPlanSummary creates a human-readable summary of the proposed schedule.
func FormatPlanSummary(plan *WeekPlan) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("\n📅 Proposed Schedule for Week of %s\n", plan.WeekStarting))
	b.WriteString(strings.Repeat("─", 55) + "\n\n")

	currentDay := ""
	for _, block := range plan.Blocks {
		if block.Day != currentDay {
			if currentDay != "" {
				b.WriteString("\n")
			}
			currentDay = block.Day
			b.WriteString(fmt.Sprintf("  %s:\n", currentDay))
		}
		b.WriteString(fmt.Sprintf("    %-20s  │  %s\n", block.TimeRange, block.Project))
		b.WriteString(fmt.Sprintf("    %-20s  │  → %s\n", "", truncate(block.NextStep, 60)))
	}

	b.WriteString(fmt.Sprintf("\n%s\n", strings.Repeat("─", 55)))
	b.WriteString(fmt.Sprintf("  Total scheduled: %.1f hrs  │  Free: %.1f hrs\n", plan.TotalHours, plan.FreeHours))

	if len(plan.Unscheduled) > 0 {
		b.WriteString(fmt.Sprintf("\n  ⚠️  %d task(s) could not be scheduled (not enough free time):\n", len(plan.Unscheduled)))
		for _, t := range plan.Unscheduled {
			b.WriteString(fmt.Sprintf("    - %s (%.1f hrs)\n", t.Project, t.EstimatedHours))
		}
	}

	return b.String()
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
