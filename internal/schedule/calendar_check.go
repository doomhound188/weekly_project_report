package schedule

import (
	"github.com/weekly-report/internal/connectwise"
)

// CheckCalendarStatus checks whether each project in the grouped entries has
// at least one ticket scheduled on the ConnectWise calendar for the upcoming week.
// Returns a map of project name → true (has scheduled ticket) / false (no scheduled ticket).
func CheckCalendarStatus(
	grouped map[string][]connectwise.TimeEntry,
	scheduleEntries []connectwise.ScheduleEntry,
) map[string]bool {
	// Build a set of ObjectIDs from the schedule entries (these reference ticket IDs)
	scheduledObjectIDs := make(map[int]bool)
	for _, entry := range scheduleEntries {
		if entry.ObjectID > 0 {
			scheduledObjectIDs[entry.ObjectID] = true
		}
	}

	result := make(map[string]bool)

	for projectName, entries := range grouped {
		found := false
		for _, e := range entries {
			if e.Ticket != nil && e.Ticket.ID > 0 {
				if scheduledObjectIDs[e.Ticket.ID] {
					found = true
					break
				}
			}
			if e.TicketDetails != nil && e.TicketDetails.ID > 0 {
				if scheduledObjectIDs[e.TicketDetails.ID] {
					found = true
					break
				}
			}
		}
		result[projectName] = found
	}

	return result
}
