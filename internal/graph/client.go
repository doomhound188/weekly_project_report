package graph

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/weekly-report/internal/config"
)

const graphBaseURL = "https://graph.microsoft.com/v1.0"

// Client interacts with the Microsoft Graph API.
type Client struct {
	clientID     string
	clientSecret string
	tenantID     string
	siteName     string
	filePath     string
	emailSender  string
	emailRecip   string
	accessToken  string
	siteID       string
	configured   bool
	http         *http.Client
}

// NewClient creates a Graph API client.
func NewClient(cfg *config.Config) *Client {
	return &Client{
		clientID:     cfg.GraphClientID,
		clientSecret: cfg.GraphClientSecret,
		tenantID:     cfg.GraphTenantID,
		siteName:     cfg.SharePointSiteName,
		filePath:     cfg.SharePointFilePath,
		emailSender:  cfg.EmailSender,
		emailRecip:   cfg.EmailRecipient,
		configured:   cfg.GraphConfigured(),
		http:         &http.Client{Timeout: 30 * time.Second},
	}
}

// IsConfigured returns whether Graph API credentials are set.
func (c *Client) IsConfigured() bool {
	return c.configured
}

func (c *Client) getAccessToken() (string, error) {
	if c.accessToken != "" {
		return c.accessToken, nil
	}

	tokenURL := fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", c.tenantID)

	data := url.Values{
		"client_id":     {c.clientID},
		"client_secret": {c.clientSecret},
		"scope":         {"https://graph.microsoft.com/.default"},
		"grant_type":    {"client_credentials"},
	}

	resp, err := c.http.PostForm(tokenURL, data)
	if err != nil {
		return "", fmt.Errorf("requesting token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("token error HTTP %d: %s", resp.StatusCode, string(body[:min(len(body), 500)]))
	}

	var result struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	c.accessToken = result.AccessToken
	return c.accessToken, nil
}

func (c *Client) authHeaders() (http.Header, error) {
	token, err := c.getAccessToken()
	if err != nil {
		return nil, err
	}
	h := http.Header{}
	h.Set("Authorization", "Bearer "+token)
	h.Set("Content-Type", "application/json")
	return h, nil
}

func (c *Client) doGet(endpoint string) ([]byte, error) {
	headers, err := c.authHeaders()
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("GET", graphBaseURL+endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header = headers

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("Graph API %d: %s", resp.StatusCode, string(body[:min(len(body), 500)]))
	}

	return body, nil
}

func (c *Client) getSiteID() (string, error) {
	if c.siteID != "" {
		return c.siteID, nil
	}

	data, err := c.doGet(fmt.Sprintf("/sites/infinityns.sharepoint.com:/sites/%s", c.siteName))
	if err != nil {
		return "", err
	}

	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return "", err
	}

	c.siteID = result.ID
	return c.siteID, nil
}

// DownloadSharePointFile downloads a file from SharePoint to outputPath.
func (c *Client) DownloadSharePointFile(outputPath string) (bool, error) {
	if !c.configured {
		return false, nil
	}

	siteID, err := c.getSiteID()
	if err != nil {
		return false, fmt.Errorf("getting site: %w", err)
	}

	// Get default drive
	driveData, err := c.doGet(fmt.Sprintf("/sites/%s/drive", siteID))
	if err != nil {
		return false, err
	}
	var drive struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(driveData, &drive); err != nil {
		return false, err
	}

	// Search for the file
	searchData, err := c.doGet(fmt.Sprintf("/drives/%s/root/search(q='%s')", drive.ID, url.PathEscape(c.filePath)))
	if err != nil {
		return false, err
	}
	var searchResult struct {
		Value []struct {
			ID          string `json:"id"`
			DownloadURL string `json:"@microsoft.graph.downloadUrl"`
		} `json:"value"`
	}
	if err := json.Unmarshal(searchData, &searchResult); err != nil {
		return false, err
	}

	if len(searchResult.Value) == 0 {
		return false, fmt.Errorf("file not found: %s", c.filePath)
	}

	item := searchResult.Value[0]
	var fileBytes []byte

	if item.DownloadURL != "" {
		resp, err := c.http.Get(item.DownloadURL)
		if err != nil {
			return false, err
		}
		defer resp.Body.Close()
		fileBytes, err = io.ReadAll(resp.Body)
		if err != nil {
			return false, err
		}
	} else {
		fileBytes, err = c.doGet(fmt.Sprintf("/drives/%s/items/%s/content", drive.ID, item.ID))
		if err != nil {
			return false, err
		}
	}

	if err := os.WriteFile(outputPath, fileBytes, 0644); err != nil {
		return false, err
	}

	return true, nil
}

// SendEmail sends an email via Microsoft Graph.
func (c *Client) SendEmail(subject, htmlBody string) (bool, error) {
	if !c.configured {
		return false, nil
	}
	if c.emailSender == "" || c.emailRecip == "" {
		return false, nil
	}

	headers, err := c.authHeaders()
	if err != nil {
		return false, err
	}

	emailData := map[string]any{
		"message": map[string]any{
			"subject": subject,
			"body": map[string]string{
				"contentType": "HTML",
				"content":     htmlBody,
			},
			"toRecipients": []map[string]any{
				{"emailAddress": map[string]string{"address": c.emailRecip}},
			},
		},
		"saveToSentItems": true,
	}

	jsonBody, _ := json.Marshal(emailData)

	reqURL := fmt.Sprintf("%s/users/%s/sendMail", graphBaseURL, c.emailSender)
	req, err := http.NewRequest("POST", reqURL, bytes.NewReader(jsonBody))
	if err != nil {
		return false, err
	}
	req.Header = headers

	resp, err := c.http.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("send email HTTP %d: %s", resp.StatusCode, string(body[:min(len(body), 500)]))
	}

	return true, nil
}

// TestConnection validates Graph API credentials.
func (c *Client) TestConnection() bool {
	if !c.configured {
		fmt.Println("⚠️  Graph API credentials not configured")
		return false
	}
	_, err := c.getAccessToken()
	if err != nil {
		fmt.Printf("❌ Graph API authentication failed: %v\n", err)
		return false
	}
	fmt.Println("✅ Graph API authentication successful")
	return true
}
