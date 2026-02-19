package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

type ProviderID string

const (
	ProviderClaude   ProviderID = "claude"
	ProviderCodex    ProviderID = "codex"
	ProviderOpenCode ProviderID = "opencode"
)

type AppState int

const (
	StateChooseProvider AppState = iota
	StateDashboard
)

type ProviderMeta struct {
	ID            ProviderID
	DisplayName   string
	HeaderTitle   string
	VersionPrefix string
	SessionLabel  string
	WeeklyLabel   string
}

var providerCatalog = []ProviderMeta{
	{
		ID:            ProviderClaude,
		DisplayName:   "Claude Code",
		HeaderTitle:   "CLAUDE CODE STATUS",
		VersionPrefix: "Claude Code",
		SessionLabel:  "Session (5h)",
		WeeklyLabel:   "Weekly (7d)",
	},
	{
		ID:            ProviderCodex,
		DisplayName:   "OpenAI Codex",
		HeaderTitle:   "OPENAI CODEX STATUS",
		VersionPrefix: "Codex CLI",
		SessionLabel:  "Session (5h)",
		WeeklyLabel:   "Weekly (7d)",
	},
	{
		ID:            ProviderOpenCode,
		DisplayName:   "OpenCode",
		HeaderTitle:   "OPENCODE STATUS",
		VersionPrefix: "OpenCode",
		SessionLabel:  "Quota (Used)",
		WeeklyLabel:   "Quota (Projected)",
	},
}

func allProviders() []ProviderMeta {
	return providerCatalog
}

func providerMeta(id ProviderID) ProviderMeta {
	for _, provider := range providerCatalog {
		if provider.ID == id {
			return provider
		}
	}
	return providerCatalog[0]
}

func providerIndex(id ProviderID) int {
	for i, provider := range providerCatalog {
		if provider.ID == id {
			return i
		}
	}
	return 0
}

func isValidProvider(id ProviderID) bool {
	for _, provider := range providerCatalog {
		if provider.ID == id {
			return true
		}
	}
	return false
}

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

// OpenCodeAuthFile represents ~/.local/share/opencode/auth.json.
type OpenCodeAuthFile struct {
	GitHubCopilot OpenCodeAuthProvider `json:"github-copilot"`
}

// OpenCodeAuthProvider holds provider-specific auth details in OpenCode auth.json.
type OpenCodeAuthProvider struct {
	OAuthToken string `json:"oauth_token"`
}

// CopilotTokenResponse represents the Copilot token exchange response.
type CopilotTokenResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// CopilotUserResponse represents the relevant fields from copilot_internal/user.
type CopilotUserResponse struct {
	QuotaSnapshots struct {
		PremiumInteractions struct {
			Remaining        int     `json:"remaining"`
			Entitlement      int     `json:"entitlement"`
			PercentRemaining float64 `json:"percent_remaining"`
		} `json:"premium_interactions"`
	} `json:"quota_snapshots"`
	QuotaResetDate string `json:"quota_reset_date"`
}

// DashboardData holds all data displayed on the dashboard.
type DashboardData struct {
	ProviderID ProviderID

	// Session usage windows
	SessionUtil    float64
	SessionResets  time.Time
	HasSessionData bool
	WeeklyUtil     float64
	WeeklyResets   time.Time
	HasWeeklyData  bool

	// OpenCode quota detail fields
	QuotaUsed        int
	QuotaEntitlement int
	QuotaProjected   int
	QuotaDaysLeft    int
	QuotaPace        string

	// ccusage - today
	DailyCost   float64
	DailyTokens int
	HasCostData bool

	// ccusage - last 30 days
	MonthlyCost    float64
	MonthlyTokens  int
	HasMonthlyData bool

	// Provider version (Claude / Codex)
	ProviderVersion string
	HasVersionData  bool

	// Metadata
	LastUpdated time.Time
	Errors      []string
}

// Bubbletea message types

type tickMsg struct{}

type dataFetchedMsg struct {
	provider ProviderID
	data     DashboardData
}

type warmupFinishedMsg struct {
	provider ProviderID
	err      error
}
