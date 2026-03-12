package connectwise

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/weekly-report/internal/config"
)

// Client interacts with the ConnectWise PSA REST API.
type Client struct {
	baseURL  string
	headers  map[string]string
	memberID string
	http     *http.Client
}

// TimeEntry represents a ConnectWise time entry.
type TimeEntry struct {
	ID          int            `json:"id"`
	DateEntered string         `json:"dateEntered"`
	ActualHours float64        `json:"actualHours"`
	Notes       string         `json:"notes"`
	Company     *Ref           `json:"company,omitempty"`
	Project     *Ref           `json:"project,omitempty"`
	Ticket      *Ref           `json:"ticket,omitempty"`
	WorkType    *Ref           `json:"workType,omitempty"`
	Member      *Ref           `json:"member,omitempty"`
	Raw         map[string]any `json:"-"`

	// Enriched fields (not from API directly)
	ProjectDetails *ProjectDetails `json:"_project_details,omitempty"`
	TicketDetails  *TicketDetails  `json:"_ticket_details,omitempty"`
}

// Ref is a generic ConnectWise reference object with id and name.
type Ref struct {
	ID   int    `json:"id"`
	Name string `json:"name,omitempty"`
}

// ProjectDetails holds fetched project info.
type ProjectDetails struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	EstimatedEnd string `json:"estimatedEnd,omitempty"`
	Company      *Ref   `json:"company,omitempty"`
}

// TicketDetails holds fetched ticket info.
type TicketDetails struct {
	ID      int    `json:"id"`
	Summary string `json:"summary"`
	Company *Ref   `json:"company,omitempty"`
}

// Member represents a ConnectWise member.
type Member struct {
	ID         int    `json:"id"`
	Identifier string `json:"identifier"`
	FirstName  string `json:"firstName"`
	LastName   string `json:"lastName"`
	Name       string `json:"name"`
	Email      string `json:"email"`
}

// NewClient creates a new ConnectWise API client.
func NewClient(cfg *config.Config) (*Client, error) {
	return NewClientForMember(cfg, cfg.CWMemberID)
}

// NewClientForMember creates a client configured for a specific member.
func NewClientForMember(cfg *config.Config, memberID string) (*Client, error) {
	if cfg.CWCompanyID == "" || cfg.CWPublicKey == "" || cfg.CWPrivateKey == "" {
		return nil, fmt.Errorf("missing required ConnectWise credentials")
	}

	authStr := fmt.Sprintf("%s+%s:%s", cfg.CWCompanyID, cfg.CWPublicKey, cfg.CWPrivateKey)
	authBytes := base64.StdEncoding.EncodeToString([]byte(authStr))

	headers := map[string]string{
		"Authorization": "Basic " + authBytes,
		"Content-Type":  "application/json",
	}
	if cfg.CWClientID != "" {
		headers["clientId"] = cfg.CWClientID
	}

	return &Client{
		baseURL:  fmt.Sprintf("https://%s/v4_6_release/apis/3.0", cfg.CWSiteURL),
		headers:  headers,
		memberID: memberID,
		http:     &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// doGet performs a GET request and decodes JSON response.
func (c *Client) doGet(endpoint string, params url.Values, result any) error {
	reqURL := c.baseURL + endpoint
	if len(params) > 0 {
		reqURL += "?" + params.Encode()
	}

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return err
	}
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error %d: %s", resp.StatusCode, string(body[:min(len(body), 500)]))
	}

	return json.NewDecoder(resp.Body).Decode(result)
}

// GetTimeEntries fetches time entries for the member from the last N days
// relative to today.
func (c *Client) GetTimeEntries(days int) ([]TimeEntry, error) {
	return c.GetTimeEntriesFromDate(days, time.Now())
}

// GetTimeEntriesFromDate fetches time entries for the member from the last N days
// ending on the given endDate.
func (c *Client) GetTimeEntriesFromDate(days int, endDate time.Time) ([]TimeEntry, error) {
	startDate := endDate.AddDate(0, 0, -days).Format("2006-01-02")
	end := endDate.Format("2006-01-02")
	conditions := fmt.Sprintf("member/identifier='%s' and dateEntered>=[%s] and dateEntered<=[%s]", c.memberID, startDate, end)

	var all []TimeEntry
	page := 1
	pageSize := 100

	for {
		params := url.Values{
			"conditions": {conditions},
			"page":       {fmt.Sprintf("%d", page)},
			"pageSize":   {fmt.Sprintf("%d", pageSize)},
			"orderBy":    {"dateEntered desc"},
		}

		var entries []TimeEntry
		if err := c.doGet("/time/entries", params, &entries); err != nil {
			return nil, fmt.Errorf("fetching time entries: %w", err)
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

// GetProject fetches project details by ID.
func (c *Client) GetProject(projectID int) (*ProjectDetails, error) {
	var p ProjectDetails
	err := c.doGet(fmt.Sprintf("/project/projects/%d", projectID), nil, &p)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// GetTicket fetches service ticket details by ID.
func (c *Client) GetTicket(ticketID int) (*TicketDetails, error) {
	var t TicketDetails
	err := c.doGet(fmt.Sprintf("/service/tickets/%d", ticketID), nil, &t)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// EnrichTimeEntries adds project/ticket details to entries with caching.
func (c *Client) EnrichTimeEntries(entries []TimeEntry) []TimeEntry {
	projectCache := make(map[int]*ProjectDetails)
	ticketCache := make(map[int]*TicketDetails)

	for i := range entries {
		// Enrich project
		if entries[i].Project != nil && entries[i].Project.ID > 0 {
			pid := entries[i].Project.ID
			if _, ok := projectCache[pid]; !ok {
				p, err := c.GetProject(pid)
				if err == nil {
					projectCache[pid] = p
				}
			}
			entries[i].ProjectDetails = projectCache[pid]
		}

		// Enrich ticket
		if entries[i].Ticket != nil && entries[i].Ticket.ID > 0 {
			tid := entries[i].Ticket.ID
			if _, ok := ticketCache[tid]; !ok {
				t, err := c.GetTicket(tid)
				if err == nil {
					ticketCache[tid] = t
				}
			}
			entries[i].TicketDetails = ticketCache[tid]
		}
	}

	return entries
}

// GetMembers fetches all active non-API members.
func (c *Client) GetMembers() ([]Member, error) {
	var all []Member
	page := 1
	pageSize := 100

	for {
		params := url.Values{
			"page":     {fmt.Sprintf("%d", page)},
			"pageSize": {fmt.Sprintf("%d", pageSize)},
			"orderBy":  {"identifier asc"},
		}

		// We need raw JSON to check licenseClass and inactiveFlag
		var raw []map[string]any
		if err := c.doGet("/system/members", params, &raw); err != nil {
			return nil, fmt.Errorf("fetching members: %w", err)
		}

		if len(raw) == 0 {
			break
		}

		for _, m := range raw {
			// Skip API accounts
			if lc, ok := m["licenseClass"].(string); ok && lc == "A" {
				continue
			}
			// Skip inactive
			if inactive, ok := m["inactiveFlag"].(bool); ok && inactive {
				continue
			}

			firstName, _ := m["firstName"].(string)
			lastName, _ := m["lastName"].(string)
			identifier, _ := m["identifier"].(string)
			email, _ := m["officeEmail"].(string)
			id := 0
			if idF, ok := m["id"].(float64); ok {
				id = int(idF)
			}

			all = append(all, Member{
				ID:         id,
				Identifier: identifier,
				FirstName:  firstName,
				LastName:   lastName,
				Name:       trim(firstName + " " + lastName),
				Email:      email,
			})
		}

		if len(raw) < pageSize {
			break
		}
		page++
	}

	return all, nil
}

// GetMemberProjects fetches all open projects where the member is the manager.
func (c *Client) GetMemberProjects(memberIdentifier string) ([]ProjectDetails, error) {
	conditions := fmt.Sprintf("manager/identifier='%s' and closedFlag=false", memberIdentifier)

	var all []ProjectDetails
	page := 1
	pageSize := 100

	for {
		params := url.Values{
			"conditions": {conditions},
			"page":       {fmt.Sprintf("%d", page)},
			"pageSize":   {fmt.Sprintf("%d", pageSize)},
			"orderBy":    {"name asc"},
		}

		var projects []ProjectDetails
		if err := c.doGet("/project/projects", params, &projects); err != nil {
			return nil, fmt.Errorf("fetching member projects: %w", err)
		}

		if len(projects) == 0 {
			break
		}

		all = append(all, projects...)
		if len(projects) < pageSize {
			break
		}
		page++
	}

	return all, nil
}

func trim(s string) string {
	// Simple trim for leading/trailing spaces
	start := 0
	end := len(s)
	for start < end && s[start] == ' ' {
		start++
	}
	for end > start && s[end-1] == ' ' {
		end--
	}
	return s[start:end]
}
