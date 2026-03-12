package connectwise

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// MemberCalendar holds the per-day shift times from a CW member's calendar.
type MemberCalendar struct {
	ID             int    `json:"id"`
	MondayStart    string `json:"mondayStartTime"`
	MondayEnd      string `json:"mondayEndTime"`
	TuesdayStart   string `json:"tuesdayStartTime"`
	TuesdayEnd     string `json:"tuesdayEndTime"`
	WednesdayStart string `json:"wednesdayStartTime"`
	WednesdayEnd   string `json:"wednesdayEndTime"`
	ThursdayStart  string `json:"thursdayStartTime"`
	ThursdayEnd    string `json:"thursdayEndTime"`
	FridayStart    string `json:"fridayStartTime"`
	FridayEnd      string `json:"fridayEndTime"`
	SaturdayStart  string `json:"saturdayStartTime"`
	SaturdayEnd    string `json:"saturdayEndTime"`
	SundayStart    string `json:"sundayStartTime"`
	SundayEnd      string `json:"sundayEndTime"`
	HolidayList    *Ref   `json:"holidayList,omitempty"`
}

// Holiday represents a single holiday from a CW holiday list.
type Holiday struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	AllDay    bool   `json:"allDayFlag"`
	Date      string `json:"date"`
	TimeStart string `json:"timeStart"`
	TimeEnd   string `json:"timeEnd"`
}

// ShiftForDay returns the start and end time strings for a given weekday.
func (mc *MemberCalendar) ShiftForDay(day time.Weekday) (start, end string) {
	switch day {
	case time.Monday:
		return mc.MondayStart, mc.MondayEnd
	case time.Tuesday:
		return mc.TuesdayStart, mc.TuesdayEnd
	case time.Wednesday:
		return mc.WednesdayStart, mc.WednesdayEnd
	case time.Thursday:
		return mc.ThursdayStart, mc.ThursdayEnd
	case time.Friday:
		return mc.FridayStart, mc.FridayEnd
	case time.Saturday:
		return mc.SaturdayStart, mc.SaturdayEnd
	case time.Sunday:
		return mc.SundayStart, mc.SundayEnd
	}
	return "09:00", "17:00"
}

// HasShift returns true if the member works on this day (has non-empty start/end).
func (mc *MemberCalendar) HasShift(day time.Weekday) bool {
	start, end := mc.ShiftForDay(day)
	return start != "" && end != ""
}

// ScheduleEntry represents a ConnectWise schedule entry.
type ScheduleEntry struct {
	ID        int      `json:"id,omitempty"`
	ObjectID  int      `json:"objectId,omitempty"`
	Member    *Ref     `json:"member,omitempty"`
	Company   *Ref     `json:"company,omitempty"`
	Type      *TypeRef `json:"type,omitempty"`
	Status    *Ref     `json:"status,omitempty"`
	Span      *SpanRef `json:"span,omitempty"`
	DateStart string   `json:"dateStart,omitempty"`
	DateEnd   string   `json:"dateEnd,omitempty"`
	Name      string   `json:"name,omitempty"`
	Reminder  *Ref     `json:"reminder,omitempty"`
}

// TypeRef is a schedule type reference.
type TypeRef struct {
	Identifier string `json:"identifier,omitempty"`
}

// SpanRef is a schedule span reference.
type SpanRef struct {
	Identifier string `json:"identifier,omitempty"`
}

// GetMemberCalendar fetches the calendar/shift data for a member by identifier.
func (c *Client) GetMemberCalendar(memberIdentifier string) (*MemberCalendar, error) {
	// First find the member to get their calendar reference
	params := url.Values{
		"conditions": {fmt.Sprintf("identifier='%s'", memberIdentifier)},
		"fields":     {"id,identifier,calendar"},
	}

	var members []struct {
		ID       int `json:"id"`
		Calendar struct {
			ID int `json:"id"`
		} `json:"calendar"`
	}
	if err := c.doGet("/system/members", params, &members); err != nil {
		return nil, fmt.Errorf("fetching member: %w", err)
	}
	if len(members) == 0 {
		return nil, fmt.Errorf("member not found: %s", memberIdentifier)
	}

	calendarID := members[0].Calendar.ID
	if calendarID == 0 {
		// Return default 9-5 if no calendar assigned
		return defaultCalendar(), nil
	}

	// Fetch calendar details
	var cal MemberCalendar
	if err := c.doGet(fmt.Sprintf("/schedule/calendars/%d", calendarID), nil, &cal); err != nil {
		return nil, fmt.Errorf("fetching calendar: %w", err)
	}

	return &cal, nil
}

// GetScheduleEntries fetches existing schedule entries for a member in a date range.
func (c *Client) GetScheduleEntries(memberIdentifier string, startDate, endDate time.Time) ([]ScheduleEntry, error) {
	start := startDate.Format("2006-01-02")
	end := endDate.Format("2006-01-02")

	conditions := fmt.Sprintf("member/identifier='%s' and dateStart>=[%s] and dateEnd<=[%s]", memberIdentifier, start, end)

	var all []ScheduleEntry
	page := 1
	pageSize := 100

	for {
		params := url.Values{
			"conditions": {conditions},
			"page":       {fmt.Sprintf("%d", page)},
			"pageSize":   {fmt.Sprintf("%d", pageSize)},
			"orderBy":    {"dateStart asc"},
		}

		var entries []ScheduleEntry
		if err := c.doGet("/schedule/entries", params, &entries); err != nil {
			return nil, fmt.Errorf("fetching schedule entries: %w", err)
		}

		if len(entries) == 0 {
			break
		}

		all = append(all, entries...)
		if len(entries) < pageSize {
			break
		}
		page++
	}

	return all, nil
}

// CreateScheduleEntry creates a new schedule entry in ConnectWise.
func (c *Client) CreateScheduleEntry(entry ScheduleEntry) (*ScheduleEntry, error) {
	jsonBody, err := json.Marshal(entry)
	if err != nil {
		return nil, err
	}

	reqURL := c.baseURL + "/schedule/entries"
	req, err := http.NewRequest("POST", reqURL, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("create schedule entry HTTP %d: %s", resp.StatusCode, string(body[:min(len(body), 500)]))
	}

	var created ScheduleEntry
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return nil, err
	}

	return &created, nil
}

func defaultCalendar() *MemberCalendar {
	return &MemberCalendar{
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
}

// GetHolidays fetches holidays from the member's calendar holiday list
// that fall within the given date range.
func (c *Client) GetHolidays(cal *MemberCalendar, startDate, endDate time.Time) ([]Holiday, error) {
	if cal.HolidayList == nil || cal.HolidayList.ID == 0 {
		return nil, nil // No holiday list configured
	}

	start := startDate.Format("2006-01-02")
	end := endDate.Format("2006-01-02")

	params := url.Values{
		"conditions": {fmt.Sprintf("date>=[%s] and date<=[%s]", start, end)},
		"pageSize":   {"25"},
	}

	endpoint := fmt.Sprintf("/schedule/holidayLists/%d/holidays", cal.HolidayList.ID)
	var holidays []Holiday
	if err := c.doGet(endpoint, params, &holidays); err != nil {
		return nil, fmt.Errorf("fetching holidays: %w", err)
	}

	return holidays, nil
}

// IsVacationOrSickDay checks if a schedule entry represents a full-day
// vacation, sick day, or other time-off that should block scheduling.
func IsVacationOrSickDay(entry ScheduleEntry) bool {
	lower := strings.ToLower(entry.Name)
	for _, keyword := range []string{"vacation", "sick", "pto", "time off", "time-off", "personal", "leave", "holiday"} {
		if strings.Contains(lower, keyword) {
			return true
		}
	}
	// Also check type identifier for time-off types
	if entry.Type != nil {
		typeID := strings.ToLower(entry.Type.Identifier)
		for _, keyword := range []string{"v", "vacation", "sick", "pto", "holiday"} {
			if typeID == keyword {
				return true
			}
		}
	}
	return false
}
