package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// FlexibleTime wraps time.Time to unmarshal from both JSON numbers (Unix ms)
// and strings (RFC3339). The credentials file uses a number, but other sources
// may use a string.
type FlexibleTime struct {
	time.Time
}

func (ft FlexibleTime) MarshalJSON() ([]byte, error) {
	if ft.Time.IsZero() {
		return []byte("null"), nil
	}
	return []byte(strconv.FormatInt(ft.Time.UnixMilli(), 10)), nil
}

func (ft *FlexibleTime) UnmarshalJSON(data []byte) error {
	s := string(data)
	if s == "null" || s == `""` {
		ft.Time = time.Time{}
		return nil
	}

	// Unquoted number → Unix milliseconds
	if s[0] != '"' {
		ms, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return fmt.Errorf("FlexibleTime: invalid number %s: %w", s, err)
		}
		ft.Time = time.UnixMilli(ms)
		return nil
	}

	// Quoted string → RFC3339
	var ts string
	if err := json.Unmarshal(data, &ts); err != nil {
		return fmt.Errorf("FlexibleTime: invalid string: %w", err)
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return fmt.Errorf("FlexibleTime: parse RFC3339 %q: %w", ts, err)
	}
	ft.Time = t
	return nil
}

// UsageWindow represents a single usage window (5-hour session or 7-day weekly).
type UsageWindow struct {
	Utilization float64   `json:"utilization"`
	ResetsAt    time.Time `json:"resets_at"`
}

// UsageData holds the parsed OAuth usage API response.
type UsageData struct {
	FiveHour UsageWindow `json:"five_hour"`
	SevenDay UsageWindow `json:"seven_day"`
}

// CcusageTotals holds the totals object from ccusage JSON output.
type CcusageTotals struct {
	TotalCost   float64 `json:"totalCost"`
	TotalTokens int     `json:"totalTokens"`
}

// CcusageDaily holds a single day's entry from the ccusage daily array.
type CcusageDaily struct {
	Date        string  `json:"date"`
	TotalCost   float64 `json:"totalCost"`
	TotalTokens int     `json:"totalTokens"`
}

// CcusageOutput holds the parsed ccusage daily JSON output.
type CcusageOutput struct {
	Totals CcusageTotals  `json:"totals"`
	Daily  []CcusageDaily `json:"daily"`
}

// OAuthCredentials holds the OAuth token fields from the credentials file.
type OAuthCredentials struct {
	AccessToken      string       `json:"accessToken"`
	RefreshToken     string       `json:"refreshToken"`
	ExpiresAt        FlexibleTime `json:"expiresAt"`
	Scopes           []string     `json:"scopes,omitempty"`
	SubscriptionType string       `json:"subscriptionType,omitempty"`
	RateLimitTier    string       `json:"rateLimitTier,omitempty"`
}

// CredentialsFile represents ~/.claude/.credentials.json.
type CredentialsFile struct {
	ClaudeAiOauth OAuthCredentials `json:"claudeAiOauth"`
}

// OAuthTokenResponse represents the OAuth2 token endpoint response.
type OAuthTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ExpiresIn    int64  `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

// DashboardData holds all data displayed on the dashboard.
type DashboardData struct {
	// OAuth usage
	SessionUtil   float64
	SessionResets time.Time
	WeeklyUtil    float64
	WeeklyResets  time.Time
	HasOAuthData  bool

	// ccusage - today
	DailyCost   float64
	DailyTokens int
	HasCostData bool

	// ccusage - last 30 days
	MonthlyCost    float64
	MonthlyTokens  int
	HasMonthlyData bool

	// Claude version
	ClaudeVersion  string
	HasVersionData bool

	// Metadata
	LastUpdated time.Time
	Errors      []string
}

// Bubbletea message types

type tickMsg struct{}

type dataFetchedMsg struct {
	data DashboardData
}
