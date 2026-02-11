package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// fetchAllData fetches data from all three sources concurrently.
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

	// ccusage cost
	go func() {
		defer wg.Done()
		cost, err := fetchCcusage()
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			data.Errors = append(data.Errors, "ccusage: "+err.Error())
			return
		}
		data.DailyCost = cost
		data.HasCostData = true
	}()

	// Stats cache
	go func() {
		defer wg.Done()
		msgs, sessions, err := fetchStatsCache()
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			data.Errors = append(data.Errors, "stats: "+err.Error())
			return
		}
		data.MessageCount = msgs
		data.SessionCount = sessions
		data.HasStatsData = true
	}()

	wg.Wait()
	data.LastUpdated = time.Now()
	return data
}

// fetchOAuthUsage reads credentials and fetches usage from the Anthropic OAuth API.
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

	token := creds.ClaudeAiOauth.AccessToken
	if token == "" {
		return nil, fmt.Errorf("no access token found")
	}

	// Check token expiry
	if !creds.ClaudeAiOauth.ExpiresAt.IsZero() && time.Now().After(creds.ClaudeAiOauth.ExpiresAt.Time) {
		return nil, fmt.Errorf("Token expired - run `claude` to refresh")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.anthropic.com/api/oauth/usage", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "claude-status/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		return nil, fmt.Errorf("Token expired - run `claude` to refresh")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("API returned %d", resp.StatusCode)
	}

	var usage UsageData
	if err := json.NewDecoder(resp.Body).Decode(&usage); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return &usage, nil
}

// fetchCcusage runs ccusage to get today's daily cost.
func fetchCcusage() (float64, error) {
	today := time.Now().Format("20060102")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "npx", "ccusage@latest", "daily", "--json", "--since", today)
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("run ccusage: %w", err)
	}

	var result CcusageOutput
	if err := json.Unmarshal(output, &result); err != nil {
		return 0, fmt.Errorf("parse ccusage: %w", err)
	}

	return result.Totals.TotalCost, nil
}

// fetchStatsCache reads today's activity stats from the Claude stats cache.
func fetchStatsCache() (messageCount int, sessionCount int, err error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return 0, 0, fmt.Errorf("home dir: %w", err)
	}

	cachePath := filepath.Join(home, ".claude", "stats-cache.json")
	data, err := os.ReadFile(cachePath)
	if err != nil {
		return 0, 0, fmt.Errorf("read stats cache: %w", err)
	}

	var cache StatsCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return 0, 0, fmt.Errorf("parse stats cache: %w", err)
	}

	today := time.Now().Format("2006-01-02")
	for _, entry := range cache.DailyActivity {
		if entry.Date == today {
			return entry.MessageCount, entry.SessionCount, nil
		}
	}

	return 0, 0, nil
}
