package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/weekly-report/internal/ai"
	"github.com/weekly-report/internal/config"
	"github.com/weekly-report/internal/connectwise"
	"github.com/weekly-report/internal/graph"
	"github.com/weekly-report/internal/report"
	"github.com/weekly-report/internal/schedule"
	"github.com/weekly-report/internal/server"
	"github.com/weekly-report/internal/sharepoint"
)

func main() {
	// CLI flags
	cli := flag.Bool("cli", false, "Run in CLI mode (for automated jobs)")
	sendEmail := flag.Bool("send-email", false, "Send report via email")
	compareProjects := flag.Bool("compare-projects", false, "Compare with SharePoint roadmap")
	mode := flag.String("mode", "manual", "Execution mode: manual or auto")
	aiProvider := flag.String("ai-provider", "", "AI provider: gemini, openai, or anthropic")
	member := flag.String("member", "", "ConnectWise member identifier")
	scheduleFlag := flag.Bool("schedule", false, "Generate schedule blocks for next week")
	testGraph := flag.Bool("test-graph", false, "Test Graph API connection")
	flag.Parse()

	// Load config
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Test Graph if requested
	if *testGraph {
		g := graph.NewClient(cfg)
		g.TestConnection()
		return
	}

	// If NOT cli mode, start web server
	if !*cli {
		srv := server.New(cfg)
		if err := srv.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Server error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// ── CLI Mode ────────────────────────────────────────────────────

	// Override member if specified
	if *member != "" {
		cfg.CWMemberID = *member
	}

	fmt.Println("==================================================")
	fmt.Println("Weekly Project Progress Report Generator")
	fmt.Println("==================================================")
	fmt.Println()

	// Validate config
	if err := cfg.ValidateCW(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Configuration Error: %v\n", err)
		os.Exit(1)
	}
	if err := cfg.ValidateAI(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Configuration Error: %v\n", err)
		os.Exit(1)
	}

	// Initialize clients
	fmt.Println("[1/6] Initializing ConnectWise client...")
	cwClient, err := connectwise.NewClient(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("      Connected as member: %s\n", cfg.CWMemberID)

	fmt.Println("[2/6] Fetching time entries from last 7 days...")
	entries, err := cwClient.GetTimeEntries(7)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("      Found %d time entries\n", len(entries))

	if len(entries) == 0 {
		fmt.Println("\n⚠️  No time entries found for the last 7 days.")
		fmt.Println("    Please check your CW_MEMBER_ID setting.")
		os.Exit(1)
	}

	fmt.Println("[3/6] Enriching entries with project/ticket details...")
	entries = cwClient.EnrichTimeEntries(entries)
	fmt.Println("      Done enriching entries")

	fmt.Println("[4/6] Organizing entries and generating AI summary...")

	initials := cfg.MemberInitials
	if *member != "" {
		initials = strings.ToUpper((*member)[:2])
	}
	gen := report.NewGenerator(initials)
	grouped := gen.GroupEntriesByProject(entries)
	fmt.Printf("      Found %d unique projects/tickets\n", len(grouped))

	summarizer, err := ai.NewSummarizer(cfg, *aiProvider)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("      Using AI provider: %s\n", summarizer.ProviderName())

	reportDate := time.Now()
	reportBody, err := summarizer.SummarizeEntries(grouped, reportDate.Format("2006-01-02"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error: %v\n", err)
		os.Exit(1)
	}

	// Compare with SharePoint roadmap
	comparisonSummary := ""
	if *compareProjects || *mode == "auto" {
		fmt.Println("[5/6] Comparing with SharePoint project roadmap...")
		graphClient := graph.NewClient(cfg)
		excelPath := "temp_roadmap.xlsx"
		ok, err := graphClient.DownloadSharePointFile(excelPath)
		if err == nil && ok {
			projects, err := sharepoint.ParseRoadmap(excelPath)
			if err == nil {
				ownerFilter := strings.ReplaceAll(cfg.CWMemberID, ".", " ")
				projectNames := make([]string, 0, len(grouped))
				for k := range grouped {
					projectNames = append(projectNames, k)
				}
				comparison := sharepoint.CompareWithTimesheet(projects, projectNames, ownerFilter)
				comparisonSummary = sharepoint.FormatComparisonSummary(comparison)
				fmt.Println(comparisonSummary)
			}
			os.Remove(excelPath)
		} else {
			fmt.Println("      Skipping comparison (SharePoint not configured)")
		}
	} else {
		fmt.Println("[5/6] Skipping SharePoint comparison (use --compare-projects to enable)")
	}

	fmt.Println("[6/6] Writing report to file...")
	subject := gen.GenerateEmailSubject(reportDate)

	fullReport := reportBody
	if comparisonSummary != "" {
		fullReport += "\n\n" + strings.Repeat("=", 50) + "\nPROJECT COMPARISON\n" + strings.Repeat("=", 50) + comparisonSummary
	}

	fpath, err := gen.WriteReport(subject, fullReport, reportDate)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("==================================================")
	fmt.Println("✅ Report generated successfully!")
	fmt.Println("==================================================")
	fmt.Println()
	fmt.Printf("📄 Report saved to: %s\n", fpath)

	// Send email
	if *sendEmail || *mode == "auto" {
		fmt.Println()
		fmt.Println("📧 Sending email...")
		graphClient := graph.NewClient(cfg)
		htmlBody := report.GenerateHTMLEmail(subject, reportBody)
		if sent, err := graphClient.SendEmail(subject, htmlBody); sent {
			fmt.Println("✅ Email sent successfully!")
		} else if err != nil {
			fmt.Printf("⚠️  Email not sent: %v\n", err)
		} else {
			fmt.Println("⚠️  Email not sent (Graph API may not be configured)")
		}
	} else {
		fmt.Println()
		fmt.Println("You can now open this file and copy the contents")
		fmt.Println("to your email client.")
		fmt.Println()
		fmt.Println("💡 Tip: Use --send-email to send automatically via Microsoft Graph")
	}

	// Schedule blocks for next week
	if *scheduleFlag {
		fmt.Println()
		fmt.Println("📅 Estimating task durations for next week...")

		estimates, err := ai.EstimateTaskDurations(cfg, reportBody, grouped)
		if err != nil {
			fmt.Fprintf(os.Stderr, "⚠️  Schedule estimation failed: %v\n", err)
		} else {
			fmt.Printf("   Estimated %d tasks\n", len(estimates))

			// Get member calendar
			calendar, err := cwClient.GetMemberCalendar(cfg.CWMemberID)
			if err != nil {
				fmt.Printf("⚠️  Could not fetch calendar, using defaults: %v\n", err)
			}

			// Get existing schedule
			now := time.Now()
			daysUntilMonday := (8 - int(now.Weekday())) % 7
			if daysUntilMonday == 0 {
				daysUntilMonday = 7
			}
			nextMonday := time.Date(now.Year(), now.Month(), now.Day()+daysUntilMonday, 0, 0, 0, 0, now.Location())
			nextFriday := nextMonday.AddDate(0, 0, 4)

			existing, _ := cwClient.GetScheduleEntries(cfg.CWMemberID, nextMonday, nextFriday.Add(24*time.Hour))

			// Fetch holidays for the target week
			holidays, _ := cwClient.GetHolidays(calendar, nextMonday, nextFriday)

			plan := schedule.PlanWeek(estimates, existing, calendar, holidays, now.Location())
			fmt.Print(schedule.FormatPlanSummary(plan))
		}
	}
}
