// Package llmproxy implements a multi-provider LLM proxy server
package llmproxy

import (
	"context"
	"time"
)

// ProviderType identifies the LLM provider
type ProviderType string

const (
	ProviderAnthropic ProviderType = "anthropic"
	ProviderOpenAI    ProviderType = "openai"
	ProviderGLM       ProviderType = "glm"
	ProviderGroq      ProviderType = "groq"
	ProviderGrok      ProviderType = "grok"
)

// String returns the string representation of the provider type
func (p ProviderType) String() string {
	return string(p)
}

// IsValid checks if the provider type is valid
func (p ProviderType) IsValid() bool {
	switch p {
	case ProviderAnthropic, ProviderOpenAI, ProviderGLM, ProviderGroq, ProviderGrok:
		return true
	default:
		return false
	}
}

// Role represents the role of a message sender
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// String returns the string representation of the role
func (r Role) String() string {
	return string(r)
}

// Message represents a chat message
type Message struct {
	Role    Role   `json:"role"`
	Content string `json:"content"`

	// Tool-related fields
	ToolCallID string `json:"tool_call_id,omitempty"`
	Name       string `json:"name,omitempty"`

	// Tool calls (for assistant messages)
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

// ToolCall represents a tool/function call
type ToolCall struct {
	ID       string          `json:"id"`
	Type     string          `json:"type"`
	Function FunctionCall    `json:"function"`
}

// FunctionCall represents a function call
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Tool represents a tool definition
type Tool struct {
	Type     string       `json:"type"`
	Function FunctionSpec `json:"function"`
}

// FunctionSpec represents a function specification
type FunctionSpec struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// ChatRequest is a request to generate a chat completion
type ChatRequest struct {
	Model            string         `json:"model"`
	Messages         []Message      `json:"messages"`
	Tools            []Tool         `json:"tools,omitempty"`
	ToolChoice       any            `json:"tool_choice,omitempty"`
	Temperature      float64        `json:"temperature,omitempty"`
	MaxTokens        int            `json:"max_tokens,omitempty"`
	TopP             float64        `json:"top_p,omitempty"`
	Stream           bool           `json:"stream,omitempty"`
	Stop             []string       `json:"stop,omitempty"`
	Provider         ProviderType   `json:"-"` // Set by proxy based on routing
	ProviderAPIKey   string         `json:"-"` // API key for the provider
	ProviderBaseURL  string         `json:"-"` // Optional custom base URL
}

// ChatResponse is the response from a chat completion
type ChatResponse struct {
	ID               string           `json:"id"`
	Object           string           `json:"object"`
	Created          int64            `json:"created"`
	Model            string           `json:"model"`
	Choices          []Choice         `json:"choices"`
	Usage            Usage            `json:"usage"`
	Provider         ProviderType     `json:"_provider,omitempty"`
	ProviderResponse any              `json:"_provider_response,omitempty"`
}

// Choice represents a choice in the response
type Choice struct {
	Index        int          `json:"index"`
	Message      Message      `json:"message"`
	FinishReason string       `json:"finish_reason"`
	Delta        *MessageDelta `json:"delta,omitempty"`
}

// MessageDelta represents a delta in a streaming response
type MessageDelta struct {
	Role      Role       `json:"role,omitempty"`
	Content   string     `json:"content,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

// Usage represents token usage information
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ChatChunk represents a chunk in a streaming response
type ChatChunk struct {
	ID               string           `json:"id"`
	Object           string           `json:"object"`
	Created          int64            `json:"created"`
	Model            string           `json:"model"`
	Choices          []Choice         `json:"choices"`
	Provider         ProviderType     `json:"_provider,omitempty"`
}

// StreamEvent is the event sent via SSE
type StreamEvent struct {
	Data  any    `json:"data"`
	Event string `json:"event,omitempty"` // Optional event type
	Error error  `json:"error,omitempty"`
}

// StreamError represents an error during streaming
type StreamError struct {
	Provider ProviderType
	Message  string
	Code     string
}

// Error returns the error message
func (e *StreamError) Error() string {
	return e.Provider.String() + ": " + e.Message + " (" + e.Code + ")"
}

// Model represents an available model
type Model struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Provider    ProviderType `json:"provider"`
	ContextSize int          `json:"context_size"`

	// Pricing (per 1M tokens)
	InputPrice  float64 `json:"input_price,omitempty"`
	OutputPrice float64 `json:"output_price,omitempty"`
}

// Cost represents cost information for a request
type Cost struct {
	InputCost     float64 `json:"input_cost"`
	OutputCost    float64 `json:"output_cost"`
	TotalCost     float64 `json:"total_cost"`
	Currency      string  `json:"currency"`
	InputTokens   int     `json:"input_tokens"`
	OutputTokens  int     `json:"output_tokens"`
	TotalTokens   int     `json:"total_tokens"`
	Model         string  `json:"model"`
	Provider      ProviderType `json:"provider"`
	Timestamp     time.Time `json:"timestamp"`
}

// LogEntry represents a logged request/response
type LogEntry struct {
	ID            string         `json:"id"`
	Timestamp     time.Time      `json:"timestamp"`
	Provider      ProviderType   `json:"provider"`
	Model         string         `json:"model"`
	RequestID     string         `json:"request_id"`
	Request       ChatRequest    `json:"request,omitempty"`
	Response      ChatResponse   `json:"response,omitempty"`
	Error         string         `json:"error,omitempty"`
	Duration      time.Duration  `json:"duration"`
	TokenUsage    Usage          `json:"token_usage,omitempty"`
	Cost          *Cost          `json:"cost,omitempty"`
	IPAddress     string         `json:"ip_address,omitempty"`
	UserAgent     string         `json:"user_agent,omitempty"`
}

// Provider is the interface that all LLM providers must implement
type Provider interface {
	// Name returns the provider name
	Name() ProviderType

	// StreamChat generates a streaming chat completion
	StreamChat(ctx context.Context, req *ChatRequest) (<-chan ChatChunk, error)

	// Chat generates a non-streaming chat completion
	Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error)

	// GetModels returns a list of available models
	GetModels() []Model

	// CalculateCost calculates the cost for a given usage
	CalculateCost(model string, usage *Usage) (*Cost, error)

	// Validate validates the provider configuration
	Validate() error
}

// ProviderConfig holds configuration for a single provider
type ProviderConfig struct {
	Type        ProviderType `json:"type" yaml:"type"`
	APIKey      string       `json:"api_key,omitempty" yaml:"api_key"`
	BaseURL     string       `json:"base_url,omitempty" yaml:"base_url"`
	Enabled     bool         `json:"enabled" yaml:"enabled"`
	ModelAlias  map[string]string `json:"model_alias,omitempty" yaml:"model_alias,omitempty"`
}

// Config holds the proxy server configuration
type Config struct {
	ListenAddr   string                    `json:"listen_addr" yaml:"listen_addr"`
	LogLevel     string                    `json:"log_level" yaml:"log_level"`
	Providers    map[ProviderType]ProviderConfig `json:"providers" yaml:"providers"`
	DefaultModel string                    `json:"default_model,omitempty" yaml:"default_model,omitempty"`
	ModelMapping map[string]ProviderType   `json:"model_mapping,omitempty" yaml:"model_mapping,omitempty"`

	// Rate limiting
	RateLimit           RateLimitConfig `json:"rate_limit,omitempty" yaml:"rate_limit,omitempty"`

	// Cost tracking
	CostBudget          CostBudgetConfig `json:"cost_budget,omitempty" yaml:"cost_budget,omitempty"`

	// Logging
	LogRequests         bool            `json:"log_requests" yaml:"log_requests"`
	LogResponses        bool            `json:"log_responses" yaml:"log_responses"`
	RedactPII           bool            `json:"redact_pii" yaml:"redact_pii"`
}

// RateLimitConfig holds rate limiting configuration
type RateLimitConfig struct {
	RequestsPerMinute int           `json:"requests_per_minute" yaml:"requests_per_minute"`
	TokensPerMinute   int           `json:"tokens_per_minute" yaml:"tokens_per_minute"`
	Enabled           bool          `json:"enabled" yaml:"enabled"`
}

// CostBudgetConfig holds cost budget configuration
type CostBudgetConfig struct {
	HourlyLimit float64 `json:"hourly_limit" yaml:"hourly_limit"`
	DailyLimit  float64 `json:"daily_limit" yaml:"daily_limit"`
	Enabled     bool    `json:"enabled" yaml:"enabled"`
}

// DefaultConfig returns a default configuration
func DefaultConfig() *Config {
	return &Config{
		ListenAddr: ":8080",
		LogLevel:   "info",
		Providers: map[ProviderType]ProviderConfig{
			ProviderAnthropic: {
				Type:    ProviderAnthropic,
				Enabled: true,
			},
			ProviderOpenAI: {
				Type:    ProviderOpenAI,
				Enabled: true,
			},
			ProviderGLM: {
				Type:    ProviderGLM,
				Enabled: false,
			},
			ProviderGroq: {
				Type:    ProviderGroq,
				Enabled: false,
			},
			ProviderGrok: {
				Type:    ProviderGrok,
				Enabled: false,
			},
		},
		RateLimit: RateLimitConfig{
			RequestsPerMinute: 100,
			TokensPerMinute:   100000,
			Enabled:           false,
		},
		CostBudget: CostBudgetConfig{
			HourlyLimit: 10.0,
			DailyLimit:  100.0,
			Enabled:     false,
		},
		LogRequests:  true,
		LogResponses: false,
		RedactPII:    false,
	}
}
