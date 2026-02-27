package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Config holds all application configuration loaded from environment variables.
type Config struct {
	// ConnectWise PSA
	CWCompanyID  string
	CWSiteURL    string
	CWPublicKey  string
	CWPrivateKey string
	CWClientID   string
	CWMemberID   string

	// AI Provider
	AIProvider   string
	AIModel      string
	GoogleAPIKey    string
	OpenAIAPIKey    string
	AnthropicAPIKey string

	// Microsoft Graph
	GraphClientID     string
	GraphClientSecret string
	GraphTenantID     string

	// SharePoint
	SharePointSiteName string
	SharePointFilePath string

	// Email
	EmailRecipient string
	EmailSender    string

	// Report
	MemberInitials string

	// Web
	WebPort    int
	WebHost    string
	SecretKey  string
	Debug      bool

	// Azure AD (optional auth)
	AzureTenantID     string
	AzureClientID     string
	AzureClientSecret string
}

// Load reads the .env file and populates a Config struct.
func Load() (*Config, error) {
	// Load .env file (ignore error if missing — env vars may be set directly)
	_ = godotenv.Load()

	port, _ := strconv.Atoi(getEnv("WEB_PORT", "5000"))

	cfg := &Config{
		CWCompanyID:  os.Getenv("CW_COMPANY_ID"),
		CWSiteURL:    getEnv("CW_SITE_URL", "api-na.myconnectwise.net"),
		CWPublicKey:  os.Getenv("CW_PUBLIC_KEY"),
		CWPrivateKey: os.Getenv("CW_PRIVATE_KEY"),
		CWClientID:   os.Getenv("CW_CLIENT_ID"),
		CWMemberID:   os.Getenv("CW_MEMBER_ID"),

		AIProvider:      getEnv("AI_PROVIDER", ""),
		AIModel:         os.Getenv("AI_MODEL"),
		GoogleAPIKey:    os.Getenv("GOOGLE_API_KEY"),
		OpenAIAPIKey:    os.Getenv("OPENAI_API_KEY"),
		AnthropicAPIKey: os.Getenv("ANTHROPIC_API_KEY"),

		GraphClientID:     os.Getenv("GRAPH_CLIENT_ID"),
		GraphClientSecret: os.Getenv("GRAPH_CLIENT_SECRET"),
		GraphTenantID:     os.Getenv("GRAPH_TENANT_ID"),

		SharePointSiteName: getEnv("SHAREPOINT_SITE_NAME", "Clients"),
		SharePointFilePath: getEnv("SHAREPOINT_FILE_PATH", "INS Project Roadmap 2025.xlsx"),

		EmailRecipient: os.Getenv("EMAIL_RECIPIENT"),
		EmailSender:    os.Getenv("EMAIL_SENDER"),

		MemberInitials: getEnv("MEMBER_INITIALS", "AN"),

		WebPort:   port,
		WebHost:   getEnv("WEB_HOST", "0.0.0.0"),
		SecretKey: os.Getenv("FLASK_SECRET_KEY"),
		Debug:     getEnv("FLASK_DEBUG", "false") == "true",

		AzureTenantID:     os.Getenv("AZURE_TENANT_ID"),
		AzureClientID:     os.Getenv("AZURE_CLIENT_ID"),
		AzureClientSecret: os.Getenv("AZURE_CLIENT_SECRET"),
	}

	return cfg, nil
}

// ValidateCW checks that required ConnectWise credentials are present.
func (c *Config) ValidateCW() error {
	if c.CWCompanyID == "" || c.CWPublicKey == "" || c.CWPrivateKey == "" || c.CWMemberID == "" {
		return fmt.Errorf("missing required ConnectWise credentials (CW_COMPANY_ID, CW_PUBLIC_KEY, CW_PRIVATE_KEY, CW_MEMBER_ID)")
	}
	return nil
}

// ValidateAI checks that at least one AI provider key is configured.
func (c *Config) ValidateAI() error {
	if c.GoogleAPIKey == "" && c.OpenAIAPIKey == "" && c.AnthropicAPIKey == "" {
		return fmt.Errorf("no AI API key configured; set GOOGLE_API_KEY, OPENAI_API_KEY, or ANTHROPIC_API_KEY")
	}
	return nil
}

// GraphConfigured returns true if Microsoft Graph credentials are present.
func (c *Config) GraphConfigured() bool {
	return c.GraphClientID != "" && c.GraphClientSecret != "" && c.GraphTenantID != ""
}

// AuthConfigured returns true if Azure AD auth credentials are present and not placeholders.
func (c *Config) AuthConfigured() bool {
	if c.AzureTenantID == "" || c.AzureClientID == "" || c.AzureClientSecret == "" {
		return false
	}
	placeholders := []string{"your_", "placeholder", "example_of", "value_here"}
	for _, p := range placeholders {
		if contains(c.AzureTenantID, p) || contains(c.AzureClientID, p) {
			return false
		}
	}
	return true
}

// Addr returns the listen address string.
func (c *Config) Addr() string {
	return fmt.Sprintf("%s:%d", c.WebHost, c.WebPort)
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
