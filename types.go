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
	TotalCost float64 `json:"totalCost"`
}

// CcusageOutput holds the parsed ccusage daily JSON output.
type CcusageOutput struct {
	Totals CcusageTotals `json:"totals"`
}

// CredentialsFile represents ~/.claude/.credentials.json.
type CredentialsFile struct {
	ClaudeAiOauth struct {
		AccessToken string `json:"accessToken"`
		ExpiresAt   FlexibleTime `json:"expiresAt"`
	} `json:"claudeAiOauth"`
}

// DailyActivity represents a single day's activity entry from stats-cache.
type DailyActivity struct {
	Date         string `json:"date"`
	MessageCount int    `json:"messageCount"`
	SessionCount int    `json:"sessionCount"`
}

// StatsCache represents the structure of ~/.claude/stats-cache.json.
type StatsCache struct {
	DailyActivity []DailyActivity `json:"dailyActivity"`
}

// DashboardData holds all data displayed on the dashboard.
type DashboardData struct {
	// OAuth usage
	SessionUtil   float64
	SessionResets time.Time
	WeeklyUtil    float64
	WeeklyResets  time.Time
	HasOAuthData  bool

	// ccusage cost
	DailyCost   float64
	HasCostData bool

	// Stats cache
	MessageCount int
	SessionCount int
	HasStatsData bool

	// Metadata
	LastUpdated time.Time
	Errors      []string
}

// Bubbletea message types

type tickMsg struct{}

type dataFetchedMsg struct {
	data DashboardData
}
