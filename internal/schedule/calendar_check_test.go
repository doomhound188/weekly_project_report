package schedule

import (
	"testing"

	"github.com/weekly-report/internal/connectwise"
)

func TestCheckCalendarStatus_MatchingTicket(t *testing.T) {
	grouped := map[string][]connectwise.TimeEntry{
		"Server Migration": {
			{Ticket: &connectwise.Ref{ID: 100}},
		},
	}

	scheduleEntries := []connectwise.ScheduleEntry{
		{ObjectID: 100, Name: "Server Migration - Phase 2"},
	}

	status := CheckCalendarStatus(grouped, scheduleEntries)

	if !status["Server Migration"] {
		t.Error("Expected 'Server Migration' to be true (ticket 100 is on calendar)")
	}
}

func TestCheckCalendarStatus_NoMatchingTicket(t *testing.T) {
	grouped := map[string][]connectwise.TimeEntry{
		"Email Setup": {
			{Ticket: &connectwise.Ref{ID: 200}},
		},
	}

	scheduleEntries := []connectwise.ScheduleEntry{
		{ObjectID: 999, Name: "Unrelated Task"},
	}

	status := CheckCalendarStatus(grouped, scheduleEntries)

	if status["Email Setup"] {
		t.Error("Expected 'Email Setup' to be false (ticket 200 is NOT on calendar)")
	}
}

func TestCheckCalendarStatus_MixedProjects(t *testing.T) {
	grouped := map[string][]connectwise.TimeEntry{
		"Project A": {
			{Ticket: &connectwise.Ref{ID: 10}},
		},
		"Project B": {
			{Ticket: &connectwise.Ref{ID: 20}},
		},
		"Project C": {
			{Ticket: &connectwise.Ref{ID: 30}},
		},
	}

	scheduleEntries := []connectwise.ScheduleEntry{
		{ObjectID: 10, Name: "Project A work"},
		{ObjectID: 30, Name: "Project C work"},
	}

	status := CheckCalendarStatus(grouped, scheduleEntries)

	if !status["Project A"] {
		t.Error("Expected 'Project A' to be true")
	}
	if status["Project B"] {
		t.Error("Expected 'Project B' to be false")
	}
	if !status["Project C"] {
		t.Error("Expected 'Project C' to be true")
	}
}

func TestCheckCalendarStatus_NoEntries(t *testing.T) {
	grouped := map[string][]connectwise.TimeEntry{
		"Project X": {
			{Ticket: &connectwise.Ref{ID: 50}},
		},
	}

	status := CheckCalendarStatus(grouped, nil)

	if status["Project X"] {
		t.Error("Expected 'Project X' to be false when no schedule entries exist")
	}
}

func TestCheckCalendarStatus_TicketDetails(t *testing.T) {
	// Test that TicketDetails.ID is also checked (not just Ticket.ID)
	grouped := map[string][]connectwise.TimeEntry{
		"Legacy Project": {
			{TicketDetails: &connectwise.TicketDetails{ID: 777}},
		},
	}

	scheduleEntries := []connectwise.ScheduleEntry{
		{ObjectID: 777, Name: "Legacy work"},
	}

	status := CheckCalendarStatus(grouped, scheduleEntries)

	if !status["Legacy Project"] {
		t.Error("Expected 'Legacy Project' to be true (TicketDetails.ID 777 matches)")
	}
}
