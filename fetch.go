package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	oauthTokenURL     = "https://console.anthropic.com/v1/oauth/token"
	oauthClientID     = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
	tokenExpiryBuffer = 5 * time.Minute
	sessionWindow     = 5 * time.Hour
	weeklyWindow      = 7 * 24 * time.Hour
	resetPastGrace    = 2 * time.Minute
	warmUpTimeout     = 45 * time.Second
	copilotTokenURL   = "https://api.github.com/copilot_internal/v2/token"
	copilotUserURL    = "https://api.github.com/copilot_internal/user"
	copilotTimeout    = 10 * time.Second

	httpErrorExcerptLimit = 512
)

var (
	errNoTokenCount      = errors.New("no token_count event found")
	errTokenCountPending = errors.New("token_count event found but rate limits are not initialized")
	warmUpCommandRunner  = runCommand

	fetchCopilotQuotaFunc        = fetchCopilotQuota
	fetchOpenCodeCcusageFunc     = fetchOpenCodeCcusage
	fetchOpenCodeVersionFunc     = fetchOpenCodeVersion
	openCodeVersionCommandRunner = runCommand
	openCodeVersionPattern       = regexp.MustCompile(`(?i)\bv?(\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?)\b`)
	readOpenCodeAuthFunc         = readOpenCodeAuth
	exchangeCopilotTokenFunc     = exchangeCopilotToken
	fetchCopilotUserFunc         = fetchCopilotUser
)

const (
	claudeWarmUpPrompt = "Reply with exactly: ok. Do not use any tools."
	codexWarmUpPrompt  = "Reply with exactly: ok. Do not run any tools."
)

// costResult holds parsed results from a single cost command call.
type costResult struct {
	dailyCost      float64
	dailyTokens    int
	hasDailyData   bool
	monthlyCost    float64
	monthlyTokens  int
	hasMonthlyData bool
}

type codexCcusageTotals struct {
	CostUSD     float64 `json:"costUSD"`
	TotalTokens int     `json:"totalTokens"`
}

type codexCcusageDaily struct {
	Date        string  `json:"date"`
	CostUSD     float64 `json:"costUSD"`
	TotalTokens int     `json:"totalTokens"`
}

type codexCcusageOutput struct {
	Totals codexCcusageTotals  `json:"totals"`
	Daily  []codexCcusageDaily `json:"daily"`
}

type codexLogLine struct {
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

type codexEventType struct {
	Type string `json:"type"`
}

type codexTokenCountPayload struct {
	Type       string          `json:"type"`
	RateLimits codexRateLimits `json:"rate_limits"`
}

type codexRateLimits struct {
	Primary   codexRateLimit `json:"primary"`
	Secondary codexRateLimit `json:"secondary"`
}

type codexRateLimit struct {
	UsedPercent   float64 `json:"used_percent"`
	WindowMinutes int     `json:"window_minutes"`
	ResetsAt      int64   `json:"resets_at"`
}

type codexStatusSnapshot struct {
	timestamp   time.Time
	hasSession  bool
	sessionUtil float64
	sessionEnds time.Time
	hasWeekly   bool
	weeklyUtil  float64
	weeklyEnds  time.Time
}

// fetchAllDataForProvider fetches provider-specific dashboard data.
func fetchAllDataForProvider(provider ProviderID) DashboardData {
	switch provider {
	case ProviderClaude:
		return fetchClaudeData()
	case ProviderCodex:
		return fetchCodexData()
	case ProviderOpenCode:
		return fetchOpenCodeData()
	default:
		return DashboardData{
			ProviderID:  provider,
			LastUpdated: time.Now(),
			Errors:      []string{fmt.Sprintf("unknown provider %q", provider)},
		}
	}
}

func fetchClaudeData() DashboardData {
	data := DashboardData{ProviderID: ProviderClaude}
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
		now := time.Now()
		if isPlausibleResetTime(now, usage.FiveHour.ResetsAt, sessionWindow) {
			data.SessionUtil = usage.FiveHour.Utilization
			data.SessionResets = usage.FiveHour.ResetsAt
			data.HasSessionData = true
		}
		if isPlausibleResetTime(now, usage.SevenDay.ResetsAt, weeklyWindow) {
			data.WeeklyUtil = usage.SevenDay.Utilization
			data.WeeklyResets = usage.SevenDay.ResetsAt
			data.HasWeeklyData = true
		}
	}()

	// ccusage cost + tokens (30-day window, extract today from daily array)
	go func() {
		defer wg.Done()
		result, err := fetchClaudeCcusage()
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
		if result.hasMonthlyData {
			data.MonthlyCost = result.monthlyCost
			data.MonthlyTokens = result.monthlyTokens
			data.HasMonthlyData = true
		}
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
		data.ProviderVersion = version
		data.HasVersionData = true
	}()

	wg.Wait()
	data.LastUpdated = time.Now()
	return data
}

func fetchCodexData() DashboardData {
	data := DashboardData{ProviderID: ProviderCodex}
	var mu sync.Mutex
	var wg sync.WaitGroup

	wg.Add(3)

	// Session and weekly status from Codex session logs.
	go func() {
		defer wg.Done()
		status, err := fetchCodexStatusFromLogs()
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			data.Errors = append(data.Errors, "status: "+err.Error())
			return
		}
		now := time.Now()
		if status.hasSession && isPlausibleResetTime(now, status.sessionEnds, sessionWindow) {
			data.SessionUtil = status.sessionUtil
			data.SessionResets = status.sessionEnds
			data.HasSessionData = true
		}
		if status.hasWeekly && isPlausibleResetTime(now, status.weeklyEnds, weeklyWindow) {
			data.WeeklyUtil = status.weeklyUtil
			data.WeeklyResets = status.weeklyEnds
			data.HasWeeklyData = true
		}
	}()

	// Codex cost + tokens from @ccusage/codex.
	go func() {
		defer wg.Done()
		result, err := fetchCodexCcusage()
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			data.Errors = append(data.Errors, "ccusage/codex: "+err.Error())
			return
		}
		if result.hasDailyData {
			data.DailyCost = result.dailyCost
			data.DailyTokens = result.dailyTokens
			data.HasCostData = true
		}
		if result.hasMonthlyData {
			data.MonthlyCost = result.monthlyCost
			data.MonthlyTokens = result.monthlyTokens
			data.HasMonthlyData = true
		}
	}()

	// Codex CLI version.
	go func() {
		defer wg.Done()
		version, err := fetchCodexVersion()
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			data.Errors = append(data.Errors, "version: "+err.Error())
			return
		}
		data.ProviderVersion = version
		data.HasVersionData = true
	}()

	wg.Wait()
	data.LastUpdated = time.Now()
	return data
}

func fetchOpenCodeData() DashboardData {
	data := DashboardData{ProviderID: ProviderOpenCode}
	var mu sync.Mutex
	var wg sync.WaitGroup

	wg.Add(3)

	// Copilot quota.
	go func() {
		defer wg.Done()
		quotaData, err := fetchCopilotQuotaFunc()
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			data.Errors = append(data.Errors, "quota: "+err.Error())
			return
		}
		if quotaData.HasSessionData {
			data.SessionUtil = quotaData.SessionUtil
			data.SessionResets = quotaData.SessionResets
			data.HasSessionData = true
		}
		if quotaData.HasWeeklyData {
			data.WeeklyUtil = quotaData.WeeklyUtil
			data.WeeklyResets = quotaData.WeeklyResets
			data.HasWeeklyData = true
		}
		if quotaData.HasSessionData || quotaData.HasWeeklyData {
			data.QuotaUsed = quotaData.QuotaUsed
			data.QuotaEntitlement = quotaData.QuotaEntitlement
			data.QuotaProjected = quotaData.QuotaProjected
			data.QuotaDaysLeft = quotaData.QuotaDaysLeft
			data.QuotaPace = quotaData.QuotaPace
		}
	}()

	// OpenCode cost + tokens from @ccusage/opencode.
	go func() {
		defer wg.Done()
		result, err := fetchOpenCodeCcusageFunc()
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			data.Errors = append(data.Errors, "ccusage/opencode: "+err.Error())
			return
		}
		if result.hasDailyData {
			data.DailyCost = result.dailyCost
			data.DailyTokens = result.dailyTokens
			data.HasCostData = true
		}
		if result.hasMonthlyData {
			data.MonthlyCost = result.monthlyCost
			data.MonthlyTokens = result.monthlyTokens
			data.HasMonthlyData = true
		}
	}()

	// OpenCode CLI version.
	go func() {
		defer wg.Done()
		version, err := fetchOpenCodeVersionFunc()
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			data.Errors = append(data.Errors, "version: "+err.Error())
			return
		}
		data.ProviderVersion = version
		data.HasVersionData = true
	}()

	wg.Wait()
	data.LastUpdated = time.Now()
	return data
}

func readOpenCodeAuth() (string, error) {
	var authPath string
	if dataDir := strings.TrimSpace(os.Getenv("OPENCODE_DATA_DIR")); dataDir != "" {
		authPath = filepath.Join(dataDir, "auth.json")
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve opencode auth path: home dir: %w", err)
		}
		authPath = filepath.Join(home, ".local", "share", "opencode", "auth.json")
	}

	raw, err := os.ReadFile(authPath)
	if err != nil {
		return "", fmt.Errorf("read opencode auth file %s: %w", authPath, err)
	}

	var authFile OpenCodeAuthFile
	if err := json.Unmarshal(raw, &authFile); err != nil {
		return "", fmt.Errorf("parse opencode auth file %s: %w", authPath, err)
	}

	oauthToken := strings.TrimSpace(authFile.GitHubCopilot.OAuthToken)
	if oauthToken == "" {
		oauthToken = strings.TrimSpace(authFile.GitHubCopilot.AccessToken)
	}
	if oauthToken == "" {
		oauthToken = strings.TrimSpace(authFile.GitHubCopilot.Access)
	}
	if oauthToken == "" {
		oauthToken = strings.TrimSpace(authFile.GitHubCopilot.Token)
	}
	if oauthToken == "" {
		return "", fmt.Errorf("opencode auth file %s missing github-copilot oauth token", authPath)
	}

	return oauthToken, nil
}

func exchangeCopilotToken(oauthToken string) (string, error) {
	if strings.TrimSpace(oauthToken) == "" {
		return "", fmt.Errorf("copilot oauth token is empty")
	}

	client := &http.Client{Timeout: copilotTimeout}
	var lastErr error
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		ctx, cancel := context.WithTimeout(context.Background(), copilotTimeout)

		req, err := http.NewRequestWithContext(ctx, method, copilotTokenURL, nil)
		if err != nil {
			cancel()
			return "", fmt.Errorf("create copilot token request: %w", err)
		}
		req.Header.Set("Authorization", "token "+oauthToken)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", fmt.Sprintf("llm-status/%s", version))

		resp, err := client.Do(req)
		if err != nil {
			cancel()
			lastErr = fmt.Errorf("copilot token request (%s): %w", method, err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			excerpt := readHTTPErrorExcerpt(resp.Body, httpErrorExcerptLimit)
			resp.Body.Close()
			cancel()
			if excerpt != "" {
				lastErr = fmt.Errorf("copilot token exchange (%s) returned %d: %s", method, resp.StatusCode, excerpt)
			} else {
				lastErr = fmt.Errorf("copilot token exchange (%s) returned %d", method, resp.StatusCode)
			}
			continue
		}

		var tokenResp CopilotTokenResponse
		if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
			resp.Body.Close()
			cancel()
			lastErr = fmt.Errorf("parse copilot token response (%s): %w", method, err)
			continue
		}
		resp.Body.Close()
		cancel()

		sessionToken := strings.TrimSpace(tokenResp.Token)
		if sessionToken == "" {
			lastErr = fmt.Errorf("copilot token response (%s) missing token", method)
			continue
		}

		return sessionToken, nil
	}

	if lastErr != nil {
		return "", lastErr
	}
	return "", fmt.Errorf("copilot token exchange failed")
}

func fetchCopilotUser(sessionToken string) (*CopilotUserResponse, error) {
	if strings.TrimSpace(sessionToken) == "" {
		return nil, fmt.Errorf("copilot session token is empty")
	}

	client := &http.Client{Timeout: copilotTimeout}
	authValues := []string{"Bearer " + sessionToken, "token " + sessionToken}
	var lastErr error
	for _, authValue := range authValues {
		ctx, cancel := context.WithTimeout(context.Background(), copilotTimeout)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, copilotUserURL, nil)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("create copilot user request: %w", err)
		}
		req.Header.Set("Authorization", authValue)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", fmt.Sprintf("llm-status/%s", version))
		req.Header.Set("Copilot-Integration-Id", "vscode-chat")

		resp, err := client.Do(req)
		if err != nil {
			cancel()
			lastErr = fmt.Errorf("copilot user request: %w", err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			excerpt := readHTTPErrorExcerpt(resp.Body, httpErrorExcerptLimit)
			resp.Body.Close()
			cancel()
			if excerpt != "" {
				lastErr = fmt.Errorf("copilot user API returned %d: %s", resp.StatusCode, excerpt)
			} else {
				lastErr = fmt.Errorf("copilot user API returned %d", resp.StatusCode)
			}
			continue
		}

		var userResp CopilotUserResponse
		if err := json.NewDecoder(resp.Body).Decode(&userResp); err != nil {
			resp.Body.Close()
			cancel()
			lastErr = fmt.Errorf("parse copilot user response: %w", err)
			continue
		}
		resp.Body.Close()
		cancel()

		return &userResp, nil
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("copilot user request failed")
}

func calculateQuotaProjection(used int, entitlement int, now time.Time) (projected int, daysLeft int, pace string) {
	if used < 0 {
		used = 0
	}
	if entitlement < 0 {
		entitlement = 0
	}

	daysElapsed := now.Day()
	if daysElapsed < 1 {
		daysElapsed = 1
	}

	year, month, _ := now.Date()
	daysInMonth := time.Date(year, month+1, 0, 0, 0, 0, 0, now.Location()).Day()

	rawProjected := int(math.Round((float64(used) / float64(daysElapsed)) * float64(daysInMonth)))
	if rawProjected < 0 {
		rawProjected = 0
	}

	daysLeft = daysInMonth - now.Day()
	if daysLeft < 0 {
		daysLeft = 0
	}

	pace = "on track"
	if rawProjected > entitlement {
		pace = "exceeding"
	}

	projected = rawProjected
	if projected > entitlement {
		projected = entitlement
	}

	return projected, daysLeft, pace
}

func fetchCopilotQuota() (DashboardData, error) {
	oauthToken, err := readOpenCodeAuthFunc()
	if err != nil {
		return DashboardData{}, err
	}

	// Prefer direct token usage first (some auth.json tokens are already valid
	// for copilot_internal/user). Fall back to exchange endpoint when needed.
	userResp, directErr := fetchCopilotUserFunc(oauthToken)
	if directErr != nil {
		sessionToken, exchangeErr := exchangeCopilotTokenFunc(oauthToken)
		if exchangeErr != nil {
			return DashboardData{}, fmt.Errorf("copilot auth failed (direct token: %v; exchange: %v)", directErr, exchangeErr)
		}
		userResp, err = fetchCopilotUserFunc(sessionToken)
		if err != nil {
			return DashboardData{}, fmt.Errorf("copilot user request with exchanged token failed: %w", err)
		}
	}

	resetAt, err := time.ParseInLocation("2006-01-02", userResp.QuotaResetDate, time.Local)
	if err != nil {
		return DashboardData{}, fmt.Errorf("parse quota reset date %q: %w", userResp.QuotaResetDate, err)
	}

	entitlement := userResp.QuotaSnapshots.PremiumInteractions.Entitlement
	if entitlement < 0 {
		entitlement = 0
	}

	used := entitlement - userResp.QuotaSnapshots.PremiumInteractions.Remaining
	if used < 0 {
		used = 0
	}

	projected, daysLeft, pace := calculateQuotaProjection(used, entitlement, time.Now())

	usedPercent := 0.0
	projectedPercent := 0.0
	if entitlement > 0 {
		usedPercent = (float64(used) / float64(entitlement)) * 100
		projectedPercent = (float64(projected) / float64(entitlement)) * 100
	}

	return DashboardData{
		ProviderID:       ProviderOpenCode,
		SessionUtil:      usedPercent,
		SessionResets:    resetAt,
		HasSessionData:   true,
		WeeklyUtil:       projectedPercent,
		WeeklyResets:     resetAt,
		HasWeeklyData:    true,
		QuotaUsed:        used,
		QuotaEntitlement: entitlement,
		QuotaProjected:   projected,
		QuotaDaysLeft:    daysLeft,
		QuotaPace:        pace,
	}, nil
}

func readHTTPErrorExcerpt(body io.Reader, limit int64) string {
	if body == nil {
		return ""
	}
	if limit <= 0 {
		limit = httpErrorExcerptLimit
	}

	// Read a bounded slice for safe, compact error reporting.
	buf, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil || len(buf) == 0 {
		return ""
	}

	truncated := int64(len(buf)) > limit
	if truncated {
		buf = buf[:limit]
	}

	excerpt := strings.Join(strings.Fields(string(buf)), " ")
	if excerpt == "" {
		return ""
	}
	if truncated {
		return excerpt + "..."
	}
	return excerpt
}

func fetchCodexStatusFromLogs() (*codexStatusSnapshot, error) {
	files, err := listCodexSessionFiles()
	if err != nil {
		return nil, err
	}
	return selectCodexStatusFromFiles(files)
}

func selectCodexStatusFromFiles(files []string) (*codexStatusSnapshot, error) {
	if len(files) == 0 {
		return nil, fmt.Errorf("no codex session files found")
	}

	ordered := append([]string(nil), files...)
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i] > ordered[j]
	})

	var lastErr error
	for i, filePath := range ordered {
		status, err := parseLatestTokenCountFromFile(filePath)
		if err == nil {
			return status, nil
		}
		if errors.Is(err, errTokenCountPending) {
			// The newest session has not emitted usable rate limits yet.
			// Avoid falling back to stale status from older sessions.
			if i == 0 {
				return &codexStatusSnapshot{}, nil
			}
			continue
		}
		if errors.Is(err, errNoTokenCount) {
			continue
		}
		lastErr = err
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("no token_count status found in ~/.codex/sessions")
}

func listCodexSessionFiles() ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("home dir: %w", err)
	}

	root := filepath.Join(home, ".codex", "sessions")
	var files []string

	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			// Skip unreadable files/directories, keep scanning.
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) == ".jsonl" {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("codex sessions directory not found")
		}
		return nil, fmt.Errorf("walk codex sessions: %w", err)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no codex session files found")
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i] > files[j]
	})
	return files, nil
}

func parseLatestTokenCountFromFile(path string) (*codexStatusSnapshot, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open codex log %s: %w", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	var latest codexStatusSnapshot
	found := false
	sawTokenCount := false

	for scanner.Scan() {
		line := scanner.Bytes()

		var record codexLogLine
		if err := json.Unmarshal(line, &record); err != nil {
			continue
		}
		if record.Type != "event_msg" || len(record.Payload) == 0 {
			continue
		}

		var evtType codexEventType
		if err := json.Unmarshal(record.Payload, &evtType); err != nil {
			continue
		}
		if evtType.Type != "token_count" {
			continue
		}
		sawTokenCount = true

		var payload codexTokenCountPayload
		if err := json.Unmarshal(record.Payload, &payload); err != nil {
			continue
		}

		snapshot := codexStatusSnapshot{}
		applyCodexRateLimits(&snapshot, payload.RateLimits)
		if !snapshot.hasSession && !snapshot.hasWeekly {
			continue
		}

		if ts, err := time.Parse(time.RFC3339Nano, record.Timestamp); err == nil {
			snapshot.timestamp = ts
		}

		if !found {
			latest = snapshot
			found = true
			continue
		}

		if latest.timestamp.IsZero() || snapshot.timestamp.After(latest.timestamp) {
			latest = snapshot
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan codex log %s: %w", path, err)
	}
	if !found {
		if sawTokenCount {
			return nil, errTokenCountPending
		}
		return nil, errNoTokenCount
	}
	return &latest, nil
}

func applyCodexRateLimits(snapshot *codexStatusSnapshot, limits codexRateLimits) {
	observed := []codexRateLimit{limits.Primary, limits.Secondary}
	for _, limit := range observed {
		switch limit.WindowMinutes {
		case 300:
			setSessionLimit(snapshot, limit)
		case 10080:
			setWeeklyLimit(snapshot, limit)
		}
	}

	if !snapshot.hasSession {
		setSessionLimit(snapshot, limits.Primary)
	}
	if !snapshot.hasWeekly {
		setWeeklyLimit(snapshot, limits.Secondary)
	}
}

func setSessionLimit(snapshot *codexStatusSnapshot, limit codexRateLimit) {
	resetAt, ok := parseResetTimestamp(limit.ResetsAt)
	if !ok || !isPlausibleResetTime(time.Now(), resetAt, sessionWindow) {
		return
	}
	snapshot.sessionUtil = limit.UsedPercent
	snapshot.sessionEnds = resetAt
	snapshot.hasSession = true
}

func setWeeklyLimit(snapshot *codexStatusSnapshot, limit codexRateLimit) {
	resetAt, ok := parseResetTimestamp(limit.ResetsAt)
	if !ok || !isPlausibleResetTime(time.Now(), resetAt, weeklyWindow) {
		return
	}
	snapshot.weeklyUtil = limit.UsedPercent
	snapshot.weeklyEnds = resetAt
	snapshot.hasWeekly = true
}

func parseResetTimestamp(raw int64) (time.Time, bool) {
	if raw <= 0 {
		return time.Time{}, false
	}
	// Guard against schema shifts between Unix seconds and Unix milliseconds.
	if raw >= 1_000_000_000_000 {
		return time.UnixMilli(raw), true
	}
	return time.Unix(raw, 0), true
}

func isPlausibleResetTime(now time.Time, resetAt time.Time, window time.Duration) bool {
	if resetAt.IsZero() {
		return false
	}
	if resetAt.Before(now.Add(-resetPastGrace)) {
		return false
	}
	maxFuture := window*2 + resetPastGrace
	if resetAt.After(now.Add(maxFuture)) {
		return false
	}
	return true
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
	perm := os.FileMode(0o600)
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
		return "", fmt.Errorf("token expired and no refresh token - run `claude` to re-authenticate")
	}

	tokenResp, err := refreshOAuthToken(creds.ClaudeAiOauth.RefreshToken)
	if err != nil {
		return "", fmt.Errorf("token refresh failed - run `claude` to re-authenticate: %w", err)
	}

	// Update credentials in-place
	creds.ClaudeAiOauth.AccessToken = tokenResp.AccessToken
	creds.ClaudeAiOauth.ExpiresAt = FlexibleTime{Time: time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)}
	if tokenResp.RefreshToken != "" {
		creds.ClaudeAiOauth.RefreshToken = tokenResp.RefreshToken
	}

	if err := saveCredentials(credPath, creds); err != nil {
		// Token is refreshed in memory even if save fails - still usable this session
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
	req.Header.Set("User-Agent", fmt.Sprintf("llm-status/%s", version))

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

func checkNpxAvailable() error {
	if _, err := exec.LookPath("npx"); err != nil {
		return errors.New("npx not found in PATH (required for cost data) - install Node.js: https://nodejs.org")
	}
	return nil
}

// fetchClaudeCcusage runs ccusage for the last 30 days and extracts today + totals.
func fetchClaudeCcusage() (*costResult, error) {
	since := time.Now().AddDate(0, 0, -30).Format("20060102")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	output, err := runCommand(ctx, "npx", "ccusage@latest", "daily", "--json", "--since", since)
	if err != nil {
		return nil, fmt.Errorf("run ccusage: %w", err)
	}

	var parsed CcusageOutput
	if err := json.Unmarshal(output, &parsed); err != nil {
		return nil, fmt.Errorf("parse ccusage: %w", err)
	}

	result := &costResult{
		monthlyCost:    parsed.Totals.TotalCost,
		monthlyTokens:  parsed.Totals.TotalTokens,
		hasMonthlyData: true,
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

// fetchCodexCcusage runs @ccusage/codex for the last 30 days and extracts today + totals.
func fetchCodexCcusage() (*costResult, error) {
	timezone := localTimezone()
	location, err := time.LoadLocation(timezone)
	if err != nil {
		timezone = "UTC"
		location = time.UTC
	}

	now := time.Now().In(location)
	since := now.AddDate(0, 0, -30).Format("20060102")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	output, err := runCommand(
		ctx,
		"npx",
		"@ccusage/codex@latest",
		"daily",
		"--json",
		"--since",
		since,
		"--timezone",
		timezone,
		"--locale",
		"en-US",
	)
	if err != nil {
		return nil, fmt.Errorf("run @ccusage/codex: %w", err)
	}

	var parsed codexCcusageOutput
	if err := json.Unmarshal(output, &parsed); err != nil {
		return nil, fmt.Errorf("parse @ccusage/codex output: %w", err)
	}

	result := &costResult{
		monthlyCost:    parsed.Totals.CostUSD,
		monthlyTokens:  parsed.Totals.TotalTokens,
		hasMonthlyData: true,
	}

	for _, entry := range parsed.Daily {
		entryDate, err := time.ParseInLocation("Jan 2, 2006", entry.Date, location)
		if err != nil {
			continue
		}
		if sameCalendarDay(entryDate, now) {
			result.dailyCost = entry.CostUSD
			result.dailyTokens = entry.TotalTokens
			result.hasDailyData = true
			break
		}
	}

	return result, nil
}

// fetchOpenCodeCcusage runs @ccusage/opencode for the last 30 days and extracts today + totals.
func fetchOpenCodeCcusage() (*costResult, error) {
	since := time.Now().AddDate(0, 0, -30).Format("20060102")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	output, err := runCommand(
		ctx,
		"npx",
		"--yes",
		"@ccusage/opencode@latest",
		"daily",
		"--json",
		"--since",
		since,
	)
	if err != nil {
		return nil, fmt.Errorf("run @ccusage/opencode: %w", err)
	}

	parsed, err := parseCcusageOutput(output)
	if err != nil {
		return nil, fmt.Errorf("parse @ccusage/opencode output: %w", err)
	}

	result := &costResult{
		monthlyCost:    parsed.Totals.TotalCost,
		monthlyTokens:  parsed.Totals.TotalTokens,
		hasMonthlyData: true,
	}

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

// parseCcusageOutput tolerates wrapper lines around JSON (for example npx notices),
// and extracts the first JSON object payload when needed.
func parseCcusageOutput(output []byte) (*CcusageOutput, error) {
	var parsed CcusageOutput
	if err := json.Unmarshal(output, &parsed); err == nil {
		return &parsed, nil
	}

	start := bytes.IndexByte(output, '{')
	end := bytes.LastIndexByte(output, '}')
	if start < 0 || end <= start {
		return nil, fmt.Errorf("no JSON object found in output")
	}

	slice := bytes.TrimSpace(output[start : end+1])
	if err := json.Unmarshal(slice, &parsed); err != nil {
		return nil, err
	}
	return &parsed, nil
}

func localTimezone() string {
	tz := strings.TrimSpace(os.Getenv("TZ"))
	if tz != "" {
		return tz
	}

	name := time.Now().Location().String()
	if name == "" || name == "Local" {
		return "UTC"
	}
	return name
}

func sameCalendarDay(a, b time.Time) bool {
	aYear, aMonth, aDay := a.Date()
	bYear, bMonth, bDay := b.Date()
	return aYear == bYear && aMonth == bMonth && aDay == bDay
}

// fetchClaudeVersion runs `claude --version` and parses the version string.
func fetchClaudeVersion() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	output, err := runCommand(ctx, "claude", "--version")
	if err != nil {
		return "", fmt.Errorf("run claude --version: %w", err)
	}

	// Output format: "2.1.39 (Claude Code)" - extract version number.
	raw := strings.TrimSpace(string(output))
	if idx := strings.Index(raw, " "); idx > 0 {
		return raw[:idx], nil
	}
	return raw, nil
}

// fetchCodexVersion runs `codex --version` and parses the version string.
func fetchCodexVersion() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	output, err := runCommand(ctx, "codex", "--version")
	if err != nil {
		return "", fmt.Errorf("run codex --version: %w", err)
	}

	raw := strings.TrimSpace(string(output))
	fields := strings.Fields(raw)
	if len(fields) >= 2 {
		return fields[1], nil
	}
	return raw, nil
}

// fetchOpenCodeVersion runs `opencode version` and parses the version string.
func fetchOpenCodeVersion() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	output, err := openCodeVersionCommandRunner(ctx, "opencode", "version")
	if err == nil {
		if parsed := parseOpenCodeVersionString(string(output)); parsed != "" {
			return parsed, nil
		}
	}

	fallbackCtx, fallbackCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer fallbackCancel()
	fallbackOutput, fallbackErr := openCodeVersionCommandRunner(fallbackCtx, "opencode", "--version")
	if fallbackErr != nil {
		if err != nil {
			return "", fmt.Errorf("run opencode version: %v; run opencode --version: %w", err, fallbackErr)
		}
		return "", fmt.Errorf("run opencode --version: %w", fallbackErr)
	}
	if parsed := parseOpenCodeVersionString(string(fallbackOutput)); parsed != "" {
		return parsed, nil
	}

	return "", fmt.Errorf("unable to parse opencode version output")
}

func parseOpenCodeVersionString(raw string) string {
	normalized := strings.TrimSpace(raw)
	if normalized == "" {
		return ""
	}
	match := openCodeVersionPattern.FindStringSubmatch(normalized)
	if len(match) != 2 {
		return ""
	}
	return match[1]
}

func warmUpProvider(provider ProviderID) error {
	switch provider {
	case ProviderClaude:
		return warmUpClaude()
	case ProviderCodex:
		return warmUpCodex()
	case ProviderOpenCode:
		return warmUpOpenCode()
	default:
		return fmt.Errorf("unknown provider %q", provider)
	}
}

func warmUpClaude() error {
	ctx, cancel := context.WithTimeout(context.Background(), warmUpTimeout)
	defer cancel()

	if _, err := warmUpCommandRunner(ctx, "claude", "-p", claudeWarmUpPrompt); err != nil {
		return fmt.Errorf("claude warm-up failed: %w", err)
	}
	return nil
}

func warmUpCodex() error {
	ctx, cancel := context.WithTimeout(context.Background(), warmUpTimeout)
	defer cancel()

	if _, err := warmUpCommandRunner(ctx, "codex", "exec", "--skip-git-repo-check", codexWarmUpPrompt); err != nil {
		return fmt.Errorf("codex warm-up failed: %w", err)
	}
	return nil
}

func warmUpOpenCode() error {
	ctx, cancel := context.WithTimeout(context.Background(), warmUpTimeout)
	defer cancel()

	if _, err := warmUpCommandRunner(ctx, "opencode", "--version"); err != nil {
		return fmt.Errorf("opencode warm-up failed: %w", err)
	}
	return nil
}

func runCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	output, err := cmd.Output()
	if err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail != "" {
			return nil, fmt.Errorf("%s %s failed: %w: %s", name, strings.Join(args, " "), err, detail)
		}
		return nil, fmt.Errorf("%s %s failed: %w", name, strings.Join(args, " "), err)
	}
	return output, nil
}
