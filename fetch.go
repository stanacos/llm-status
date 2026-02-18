package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	oauthTokenURL     = "https://console.anthropic.com/v1/oauth/token"
	oauthClientID     = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
	tokenExpiryBuffer = 5 * time.Minute
)

// ccusageResult holds parsed results from a single ccusage call.
type ccusageResult struct {
	dailyCost    float64
	dailyTokens  int
	hasDailyData bool
	monthlyCost  float64
	monthlyTokens int
}

// fetchAllData fetches data from all sources concurrently.
func fetchAllData() DashboardData {
	var data DashboardData
	var mu sync.Mutex
	var wg sync.WaitGroup

	wg.Add(3)

	// OAuth usage
	go func() {
		defer wg.Done()
		usage, err := fetchOAuthUsage()
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			data.Errors = append(data.Errors, "OAuth: "+err.Error())
			return
		}
		data.SessionUtil = usage.FiveHour.Utilization
		data.SessionResets = usage.FiveHour.ResetsAt
		data.WeeklyUtil = usage.SevenDay.Utilization
		data.WeeklyResets = usage.SevenDay.ResetsAt
		data.HasOAuthData = true
	}()

	// ccusage cost + tokens (30-day window, extract today from daily array)
	go func() {
		defer wg.Done()
		result, err := fetchCcusage()
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			data.Errors = append(data.Errors, "ccusage: "+err.Error())
			return
		}
		if result.hasDailyData {
			data.DailyCost = result.dailyCost
			data.DailyTokens = result.dailyTokens
			data.HasCostData = true
		}
		data.MonthlyCost = result.monthlyCost
		data.MonthlyTokens = result.monthlyTokens
		data.HasMonthlyData = true
	}()

	// Claude Code version
	go func() {
		defer wg.Done()
		version, err := fetchClaudeVersion()
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			data.Errors = append(data.Errors, "version: "+err.Error())
			return
		}
		data.ClaudeVersion = version
		data.HasVersionData = true
	}()

	wg.Wait()
	data.LastUpdated = time.Now()
	return data
}

// refreshOAuthToken exchanges a refresh token for a new access token.
func refreshOAuthToken(refreshToken string) (*OAuthTokenResponse, error) {
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {oauthClientID},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", oauthTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("refresh request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token refresh returned %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp OAuthTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("parse token response: %w", err)
	}
	return &tokenResp, nil
}

// saveCredentials atomically writes the credentials file to disk.
func saveCredentials(credPath string, creds *CredentialsFile) error {
	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal credentials: %w", err)
	}
	data = append(data, '\n')

	// Preserve original file permissions
	perm := os.FileMode(0600)
	if info, err := os.Stat(credPath); err == nil {
		perm = info.Mode().Perm()
	}

	// Atomic write: temp file + rename
	dir := filepath.Dir(credPath)
	tmp, err := os.CreateTemp(dir, ".credentials-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Chmod(tmpPath, perm); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := os.Rename(tmpPath, credPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename temp file: %w", err)
	}
	return nil
}

// ensureValidToken checks if the token is still valid and refreshes it if needed.
// Returns the access token to use.
func ensureValidToken(credPath string, creds *CredentialsFile) (string, error) {
	// If token hasn't expired (with buffer), return it as-is
	if !creds.ClaudeAiOauth.ExpiresAt.IsZero() &&
		time.Now().Before(creds.ClaudeAiOauth.ExpiresAt.Time.Add(-tokenExpiryBuffer)) {
		return creds.ClaudeAiOauth.AccessToken, nil
	}

	return refreshAndSave(credPath, creds)
}

// refreshAndSave forces a token refresh and saves the updated credentials.
func refreshAndSave(credPath string, creds *CredentialsFile) (string, error) {
	if creds.ClaudeAiOauth.RefreshToken == "" {
		return "", fmt.Errorf("token expired and no refresh token — run `claude` to re-authenticate")
	}

	tokenResp, err := refreshOAuthToken(creds.ClaudeAiOauth.RefreshToken)
	if err != nil {
		return "", fmt.Errorf("token refresh failed — run `claude` to re-authenticate: %w", err)
	}

	// Update credentials in-place
	creds.ClaudeAiOauth.AccessToken = tokenResp.AccessToken
	creds.ClaudeAiOauth.ExpiresAt = FlexibleTime{Time: time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)}
	if tokenResp.RefreshToken != "" {
		creds.ClaudeAiOauth.RefreshToken = tokenResp.RefreshToken
	}

	if err := saveCredentials(credPath, creds); err != nil {
		// Token is refreshed in memory even if save fails — still usable this session
		return creds.ClaudeAiOauth.AccessToken, nil
	}

	return creds.ClaudeAiOauth.AccessToken, nil
}

// doUsageRequest performs the HTTP GET to the usage API and returns the parsed data.
func doUsageRequest(token string) (*UsageData, int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.anthropic.com/api/oauth/usage", nil)
	if err != nil {
		return nil, 0, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "claude-status/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("API request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, resp.StatusCode, fmt.Errorf("API returned %d", resp.StatusCode)
	}

	var usage UsageData
	if err := json.NewDecoder(resp.Body).Decode(&usage); err != nil {
		return nil, resp.StatusCode, fmt.Errorf("parse response: %w", err)
	}
	return &usage, resp.StatusCode, nil
}

// fetchOAuthUsage reads credentials and fetches usage from the Anthropic OAuth API.
// Automatically refreshes expired tokens using the OAuth2 refresh_token grant.
func fetchOAuthUsage() (*UsageData, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("home dir: %w", err)
	}

	credPath := filepath.Join(home, ".claude", ".credentials.json")
	credData, err := os.ReadFile(credPath)
	if err != nil {
		return nil, fmt.Errorf("read credentials: %w", err)
	}

	var creds CredentialsFile
	if err := json.Unmarshal(credData, &creds); err != nil {
		return nil, fmt.Errorf("parse credentials: %w", err)
	}

	if creds.ClaudeAiOauth.AccessToken == "" {
		return nil, fmt.Errorf("no access token found")
	}

	// Proactively refresh if token is expired or near-expiry
	token, err := ensureValidToken(credPath, &creds)
	if err != nil {
		return nil, err
	}

	// Call the usage API
	usage, statusCode, err := doUsageRequest(token)
	if err == nil {
		return usage, nil
	}

	// On 401, try one more refresh + retry (clock skew, server-side revocation, etc.)
	if statusCode == 401 {
		newToken, refreshErr := refreshAndSave(credPath, &creds)
		if refreshErr != nil {
			return nil, refreshErr
		}
		usage, _, err = doUsageRequest(newToken)
		if err != nil {
			return nil, err
		}
		return usage, nil
	}

	return nil, err
}

// fetchCcusage runs ccusage for the last 30 days and extracts today + totals.
func fetchCcusage() (*ccusageResult, error) {
	since := time.Now().AddDate(0, 0, -30).Format("20060102")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "npx", "ccusage@latest", "daily", "--json", "--since", since)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("run ccusage: %w", err)
	}

	var parsed CcusageOutput
	if err := json.Unmarshal(output, &parsed); err != nil {
		return nil, fmt.Errorf("parse ccusage: %w", err)
	}

	result := &ccusageResult{
		monthlyCost:   parsed.Totals.TotalCost,
		monthlyTokens: parsed.Totals.TotalTokens,
	}

	// Find today's entry in the daily array
	today := time.Now().Format("2006-01-02")
	for _, entry := range parsed.Daily {
		if entry.Date == today {
			result.dailyCost = entry.TotalCost
			result.dailyTokens = entry.TotalTokens
			result.hasDailyData = true
			break
		}
	}

	return result, nil
}

// fetchClaudeVersion runs `claude --version` and parses the version string.
func fetchClaudeVersion() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "claude", "--version")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("run claude --version: %w", err)
	}

	// Output format: "2.1.39 (Claude Code)\n" — extract version number
	raw := strings.TrimSpace(string(output))
	if idx := strings.Index(raw, " "); idx > 0 {
		return raw[:idx], nil
	}
	return raw, nil
}
