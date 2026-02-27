package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/weekly-report/internal/ai"
	"github.com/weekly-report/internal/auth"
	"github.com/weekly-report/internal/config"
	"github.com/weekly-report/internal/connectwise"
	"github.com/weekly-report/internal/graph"
	"github.com/weekly-report/internal/report"
	"github.com/weekly-report/internal/schedule"
	"github.com/weekly-report/internal/sharepoint"
)

// Server is the HTTP server for the web interface.
type Server struct {
	cfg         *config.Config
	authHandler *auth.Handler
	graphClient *graph.Client
	mux         *http.ServeMux
}

// New creates a new HTTP server.
func New(cfg *config.Config) *Server {
	s := &Server{
		cfg:         cfg,
		authHandler: auth.NewHandler(cfg),
		graphClient: graph.NewClient(cfg),
		mux:         http.NewServeMux(),
	}

	s.registerRoutes()
	return s
}

func (s *Server) registerRoutes() {
	// Auth routes
	s.mux.HandleFunc("GET /auth/login", s.authHandler.LoginHandler)
	s.mux.HandleFunc("GET /auth/callback", s.authHandler.CallbackHandler)
	s.mux.HandleFunc("GET /auth/logout", s.authHandler.LogoutHandler)
	s.mux.HandleFunc("GET /auth/user", s.authHandler.UserInfoHandler)

	// API routes (protected)
	s.mux.Handle("GET /api/members", s.authHandler.RequireAuth(http.HandlerFunc(s.membersHandler)))
	s.mux.Handle("POST /api/generate", s.authHandler.RequireAuth(http.HandlerFunc(s.generateHandler)))
	s.mux.Handle("POST /api/download", s.authHandler.RequireAuth(http.HandlerFunc(s.downloadHandler)))
	s.mux.Handle("POST /api/schedule", s.authHandler.RequireAuth(http.HandlerFunc(s.scheduleHandler)))
	s.mux.Handle("POST /api/schedule/confirm", s.authHandler.RequireAuth(http.HandlerFunc(s.scheduleConfirmHandler)))
	s.mux.HandleFunc("GET /api/health", s.healthHandler)

	// Static files from web/ directory
	webDir := findWebDir()
	fs := http.FileServer(http.Dir(webDir))
	s.mux.Handle("GET /", s.authHandler.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Serve index.html for root path
		if r.URL.Path == "/" {
			http.ServeFile(w, r, filepath.Join(webDir, "index.html"))
			return
		}
		fs.ServeHTTP(w, r)
	})))
}

// Start begins listening on the configured address.
func (s *Server) Start() error {
	addr := s.cfg.Addr()
	fmt.Println("==================================================")
	fmt.Println("Weekly Project Report Generator - Web Interface")
	fmt.Println("==================================================")
	fmt.Printf("🚀 Server listening at http://%s\n", addr)
	fmt.Println("Press CTRL+C to quit")
	fmt.Println()

	return http.ListenAndServe(addr, s.mux)
}

// membersHandler returns the list of ConnectWise members.
func (s *Server) membersHandler(w http.ResponseWriter, r *http.Request) {
	client, err := connectwise.NewClient(s.cfg)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	members, err := client.GetMembers()
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	jsonResponse(w, members)
}

// GenerateRequest is the request body for report generation.
type GenerateRequest struct {
	MemberID        string `json:"member_id"`
	Days            int    `json:"days"`
	EndDate         string `json:"end_date"`
	CompareProjects bool   `json:"compare_projects"`
	SendEmail       bool   `json:"send_email"`
}

// GenerateResponse is the response from report generation.
type GenerateResponse struct {
	Success           bool   `json:"success"`
	ReportBody        string `json:"report_body,omitempty"`
	ReportSubject     string `json:"report_subject,omitempty"`
	ComparisonSummary string `json:"comparison_summary,omitempty"`
	EmailSent         bool   `json:"email_sent"`
	AIProvider        string `json:"ai_provider,omitempty"`
	EntryCount        int    `json:"entry_count"`
	ProjectCount      int    `json:"project_count"`
	Error             string `json:"error,omitempty"`
}

func (s *Server) generateHandler(w http.ResponseWriter, r *http.Request) {
	var req GenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.MemberID == "" {
		jsonError(w, "member_id is required", http.StatusBadRequest)
		return
	}
	if req.Days <= 0 {
		req.Days = 7
	}

	// Parse end date (defaults to today)
	reportDate := time.Now()
	if req.EndDate != "" {
		parsed, err := time.Parse("2006-01-02", req.EndDate)
		if err != nil {
			jsonError(w, "Invalid end_date format, expected YYYY-MM-DD", http.StatusBadRequest)
			return
		}
		reportDate = parsed
	}

	// Create a CW client for this specific member
	origMember := s.cfg.CWMemberID
	s.cfg.CWMemberID = req.MemberID
	defer func() { s.cfg.CWMemberID = origMember }()

	cwClient, err := connectwise.NewClientForMember(s.cfg, req.MemberID)
	if err != nil {
		jsonResponse(w, GenerateResponse{Success: false, Error: err.Error()})
		return
	}

	// Fetch time entries
	entries, err := cwClient.GetTimeEntriesFromDate(req.Days, reportDate)
	if err != nil {
		jsonResponse(w, GenerateResponse{Success: false, Error: err.Error()})
		return
	}
	if len(entries) == 0 {
		jsonResponse(w, GenerateResponse{
			Success: false,
			Error:   fmt.Sprintf("No time entries found for member '%s' in the last %d days.", req.MemberID, req.Days),
		})
		return
	}

	// Enrich entries
	entries = cwClient.EnrichTimeEntries(entries)

	// Group entries
	initials := strings.ToUpper(req.MemberID[:2])
	if origMember == req.MemberID && s.cfg.MemberInitials != "" {
		initials = s.cfg.MemberInitials
	}

	gen := report.NewGenerator(initials)
	grouped := gen.GroupEntriesByProject(entries)

	// AI summarize
	summarizer, err := ai.NewSummarizer(s.cfg, "")
	if err != nil {
		jsonResponse(w, GenerateResponse{Success: false, Error: err.Error()})
		return
	}

	reportBody, err := summarizer.SummarizeEntries(grouped, reportDate.Format("2006-01-02"))
	if err != nil {
		jsonResponse(w, GenerateResponse{Success: false, Error: err.Error()})
		return
	}

	subject := gen.GenerateEmailSubject(reportDate)

	// Compare with SharePoint if requested
	var comparisonSummary string
	if req.CompareProjects {
		excelPath := filepath.Join(os.TempDir(), "temp_roadmap.xlsx")
		ok, err := s.graphClient.DownloadSharePointFile(excelPath)
		if err == nil && ok {
			projects, err := sharepoint.ParseRoadmap(excelPath)
			if err == nil {
				ownerFilter := strings.ReplaceAll(req.MemberID, ".", " ")
				projectNames := make([]string, 0, len(grouped))
				for k := range grouped {
					projectNames = append(projectNames, k)
				}
				comparison := sharepoint.CompareWithTimesheet(projects, projectNames, ownerFilter)
				comparisonSummary = sharepoint.FormatComparisonSummary(comparison)
			}
			os.Remove(excelPath)
		}
	}

	// Send email if requested
	emailSent := false
	if req.SendEmail {
		htmlBody := report.GenerateHTMLEmail(subject, reportBody)
		emailSent, _ = s.graphClient.SendEmail(subject, htmlBody)
	}

	jsonResponse(w, GenerateResponse{
		Success:           true,
		ReportBody:        reportBody,
		ReportSubject:     subject,
		ComparisonSummary: comparisonSummary,
		EmailSent:         emailSent,
		AIProvider:        summarizer.ProviderName(),
		EntryCount:        len(entries),
		ProjectCount:      len(grouped),
	})
}

func (s *Server) downloadHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ReportText    string `json:"report_text"`
		ReportSubject string `json:"report_subject"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	filename := fmt.Sprintf("weekly_report_%s.txt", time.Now().Format("2006-01-02"))
	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	w.Write([]byte(req.ReportText))
}

func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, map[string]string{
		"status":  "healthy",
		"service": "weekly-report-generator",
	})
}

// ScheduleRequest is the request body for schedule proposal.
type ScheduleRequest struct {
	MemberID   string `json:"member_id"`
	ReportBody string `json:"report_body"`
}

func (s *Server) scheduleHandler(w http.ResponseWriter, r *http.Request) {
	var req ScheduleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.MemberID == "" || req.ReportBody == "" {
		jsonError(w, "member_id and report_body are required", http.StatusBadRequest)
		return
	}

	// Create CW client
	cwClient, err := connectwise.NewClientForMember(s.cfg, req.MemberID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Get AI estimates
	estimates, err := ai.EstimateTaskDurations(s.cfg, req.ReportBody, nil)
	if err != nil {
		jsonError(w, "AI estimation failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Get member's shift calendar
	calendar, err := cwClient.GetMemberCalendar(req.MemberID)
	if err != nil {
		jsonError(w, "Failed to fetch member calendar: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Get existing schedule for next week
	now := time.Now()
	daysUntilMonday := (8 - int(now.Weekday())) % 7
	if daysUntilMonday == 0 {
		daysUntilMonday = 7
	}
	nextMonday := time.Date(now.Year(), now.Month(), now.Day()+daysUntilMonday, 0, 0, 0, 0, now.Location())
	nextFriday := nextMonday.AddDate(0, 0, 4)

	existing, err := cwClient.GetScheduleEntries(req.MemberID, nextMonday, nextFriday.Add(24*time.Hour))
	if err != nil {
		existing = nil // Non-fatal - proceed without conflict detection
	}

	// Fetch holidays for the target week
	holidays, _ := cwClient.GetHolidays(calendar, nextMonday, nextFriday)

	// Generate plan
	plan := schedule.PlanWeek(estimates, existing, calendar, holidays, now.Location())

	jsonResponse(w, plan)
}

func (s *Server) scheduleConfirmHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MemberID string                   `json:"member_id"`
		Blocks   []schedule.ProposedBlock `json:"blocks"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	cwClient, err := connectwise.NewClientForMember(s.cfg, req.MemberID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var created []int
	var errors []string

	for _, block := range req.Blocks {
		entry := connectwise.ScheduleEntry{
			ObjectID:  block.TicketID,
			Member:    &connectwise.Ref{Name: req.MemberID},
			Type:      &connectwise.TypeRef{Identifier: "S"},
			DateStart: block.DateStart.Format(time.RFC3339),
			DateEnd:   block.DateEnd.Format(time.RFC3339),
			Name:      block.Project + " - " + truncateStr(block.NextStep, 80),
		}

		result, err := cwClient.CreateScheduleEntry(entry)
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", block.Project, err))
		} else {
			created = append(created, result.ID)
		}
	}

	jsonResponse(w, map[string]any{
		"created_count": len(created),
		"created_ids":   created,
		"errors":        errors,
	})
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func jsonResponse(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func findWebDir() string {
	// Look for web/ directory relative to executable or CWD
	candidates := []string{
		"web",
		filepath.Join(execDir(), "web"),
	}
	for _, dir := range candidates {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir
		}
	}
	return "web" // fallback
}

func execDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}
