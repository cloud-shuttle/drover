package llmproxy

import (
	"testing"
	"time"
)

func TestProviderTypeString(t *testing.T) {
	tests := []struct {
		name     string
		provider ProviderType
		expected string
	}{
		{"Anthropic", ProviderAnthropic, "anthropic"},
		{"OpenAI", ProviderOpenAI, "openai"},
		{"GLM", ProviderGLM, "glm"},
		{"Groq", ProviderGroq, "groq"},
		{"Grok", ProviderGrok, "grok"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.provider.String(); got != tt.expected {
				t.Errorf("ProviderType.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestProviderTypeIsValid(t *testing.T) {
	tests := []struct {
		name     string
		provider ProviderType
		expected bool
	}{
		{"Anthropic", ProviderAnthropic, true},
		{"OpenAI", ProviderOpenAI, true},
		{"GLM", ProviderGLM, true},
		{"Groq", ProviderGroq, true},
		{"Grok", ProviderGrok, true},
		{"Invalid", ProviderType("invalid"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.provider.IsValid(); got != tt.expected {
				t.Errorf("ProviderType.IsValid() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestRoleString(t *testing.T) {
	tests := []struct {
		name     string
		role     Role
		expected string
	}{
		{"System", RoleSystem, "system"},
		{"User", RoleUser, "user"},
		{"Assistant", RoleAssistant, "assistant"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.role.String(); got != tt.expected {
				t.Errorf("Role.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestMessageCreation(t *testing.T) {
	msg := Message{
		Role:    RoleUser,
		Content: "Hello, world!",
	}

	if msg.Role != RoleUser {
		t.Errorf("Message.Role = %v, want %v", msg.Role, RoleUser)
	}

	if msg.Content != "Hello, world!" {
		t.Errorf("Message.Content = %v, want 'Hello, world!'", msg.Content)
	}
}

func TestChatRequestCreation(t *testing.T) {
	req := ChatRequest{
		Messages: []Message{
			{Role: RoleSystem, Content: "You are a helpful assistant."},
			{Role: RoleUser, Content: "Hello!"},
		},
		Model:       "gpt-4",
		MaxTokens:   1000,
		Temperature: 0.7,
		Stream:      true,
	}

	if len(req.Messages) != 2 {
		t.Errorf("ChatRequest.Messages length = %v, want 2", len(req.Messages))
	}

	if req.Model != "gpt-4" {
		t.Errorf("ChatRequest.Model = %v, want 'gpt-4'", req.Model)
	}

	if req.MaxTokens != 1000 {
		t.Errorf("ChatRequest.MaxTokens = %v, want 1000", req.MaxTokens)
	}

	if req.Temperature != 0.7 {
		t.Errorf("ChatRequest.Temperature = %v, want 0.7", req.Temperature)
	}

	if !req.Stream {
		t.Error("ChatRequest.Stream should be true")
	}
}

func TestChatResponseCreation(t *testing.T) {
	now := time.Now()
	resp := ChatResponse{
		ID:      "chatcmpl-123",
		Model:   "gpt-4",
		Choices: []Choice{
			{
				Index: 0,
				Message: Message{
					Role:    RoleAssistant,
					Content: "Hello! How can I help you?",
				},
				FinishReason: "stop",
			},
		},
		Usage: Usage{
			PromptTokens:     10,
			CompletionTokens: 20,
			TotalTokens:      30,
		},
		Created: now.Unix(),
	}

	if resp.ID != "chatcmpl-123" {
		t.Errorf("ChatResponse.ID = %v, want 'chatcmpl-123'", resp.ID)
	}

	if resp.Model != "gpt-4" {
		t.Errorf("ChatResponse.Model = %v, want 'gpt-4'", resp.Model)
	}

	if len(resp.Choices) != 1 {
		t.Fatalf("ChatResponse.Choices length = %v, want 1", len(resp.Choices))
	}

	choice := resp.Choices[0]
	if choice.Index != 0 {
		t.Errorf("Choice.Index = %v, want 0", choice.Index)
	}

	if choice.Message.Role != RoleAssistant {
		t.Errorf("Choice.Message.Role = %v, want %v", choice.Message.Role, RoleAssistant)
	}

	if choice.FinishReason != "stop" {
		t.Errorf("Choice.FinishReason = %v, want 'stop'", choice.FinishReason)
	}

	if resp.Usage.PromptTokens != 10 {
		t.Errorf("ChatResponse.Usage.PromptTokens = %v, want 10", resp.Usage.PromptTokens)
	}

	if resp.Usage.TotalTokens != 30 {
		t.Errorf("ChatResponse.Usage.TotalTokens = %v, want 30", resp.Usage.TotalTokens)
	}

	if resp.Created != now.Unix() {
		t.Errorf("ChatResponse.Created = %v, want %v", resp.Created, now.Unix())
	}
}

func TestModelCreation(t *testing.T) {
	model := Model{
		ID:          "gpt-4",
		Name:        "GPT-4",
		Provider:    ProviderOpenAI,
		ContextSize: 8192,
	}

	if model.ID != "gpt-4" {
		t.Errorf("Model.ID = %v, want 'gpt-4'", model.ID)
	}

	if model.Name != "GPT-4" {
		t.Errorf("Model.Name = %v, want 'GPT-4'", model.Name)
	}

	if model.Provider != ProviderOpenAI {
		t.Errorf("Model.Provider = %v, want %v", model.Provider, ProviderOpenAI)
	}

	if model.ContextSize != 8192 {
		t.Errorf("Model.ContextSize = %v, want 8192", model.ContextSize)
	}
}

func TestConfigDefaults(t *testing.T) {
	cfg := DefaultConfig()

	if cfg == nil {
		t.Fatal("DefaultConfig() returned nil")
	}

	if cfg.ListenAddr != ":8080" {
		t.Errorf("DefaultConfig() ListenAddr = %v, want ':8080'", cfg.ListenAddr)
	}

	if cfg.LogLevel != "info" {
		t.Errorf("DefaultConfig() LogLevel = %v, want 'info'", cfg.LogLevel)
	}

	if len(cfg.Providers) != 5 {
		t.Errorf("DefaultConfig() Providers length = %v, want 5", len(cfg.Providers))
	}

	if cfg.CostBudget.HourlyLimit != 10.0 {
		t.Errorf("DefaultConfig() CostBudget.HourlyLimit = %v, want 10.0", cfg.CostBudget.HourlyLimit)
	}

	if cfg.CostBudget.DailyLimit != 100.0 {
		t.Errorf("DefaultConfig() CostBudget.DailyLimit = %v, want 100.0", cfg.CostBudget.DailyLimit)
	}

	if cfg.RateLimit.RequestsPerMinute != 100 {
		t.Errorf("DefaultConfig() RateLimit.RequestsPerMinute = %v, want 100", cfg.RateLimit.RequestsPerMinute)
	}

	if cfg.RateLimit.TokensPerMinute != 100000 {
		t.Errorf("DefaultConfig() RateLimit.TokensPerMinute = %v, want 100000", cfg.RateLimit.TokensPerMinute)
	}
}

func TestCostCreation(t *testing.T) {
	cost := Cost{
		Provider:     ProviderOpenAI,
		Model:        "gpt-4",
		InputCost:    0.03,
		OutputCost:   0.06,
		TotalCost:    0.09,
		Currency:     "USD",
		InputTokens:  100,
		OutputTokens: 200,
		TotalTokens:  300,
		Timestamp:    time.Now(),
	}

	if cost.Provider != ProviderOpenAI {
		t.Errorf("Cost.Provider = %v, want %v", cost.Provider, ProviderOpenAI)
	}

	if cost.Model != "gpt-4" {
		t.Errorf("Cost.Model = %v, want 'gpt-4'", cost.Model)
	}

	if cost.InputCost != 0.03 {
		t.Errorf("Cost.InputCost = %v, want 0.03", cost.InputCost)
	}

	if cost.OutputCost != 0.06 {
		t.Errorf("Cost.OutputCost = %v, want 0.06", cost.OutputCost)
	}

	if cost.TotalCost != 0.09 {
		t.Errorf("Cost.TotalCost = %v, want 0.09", cost.TotalCost)
	}

	if cost.Currency != "USD" {
		t.Errorf("Cost.Currency = %v, want 'USD'", cost.Currency)
	}

	if cost.InputTokens != 100 {
		t.Errorf("Cost.InputTokens = %v, want 100", cost.InputTokens)
	}

	if cost.OutputTokens != 200 {
		t.Errorf("Cost.OutputTokens = %v, want 200", cost.OutputTokens)
	}

	if cost.TotalTokens != 300 {
		t.Errorf("Cost.TotalTokens = %v, want 300", cost.TotalTokens)
	}
}

func TestLogEntryCreation(t *testing.T) {
	req := &ChatRequest{
		Messages: []Message{
			{Role: RoleUser, Content: "Test message"},
		},
		Model: "gpt-4",
	}

	resp := &ChatResponse{
		ID:    "chatcmpl-123",
		Model: "gpt-4",
	}

	entry := LogEntry{
		Timestamp:  time.Now(),
		Provider:   ProviderOpenAI,
		Model:      "gpt-4",
		RequestID:  "req-123",
		Request:    *req,
		Response:   *resp,
		Duration:   100 * time.Millisecond,
		IPAddress:  "127.0.0.1",
		UserAgent:  "test-agent",
	}

	if entry.RequestID != "req-123" {
		t.Errorf("LogEntry.RequestID = %v, want 'req-123'", entry.RequestID)
	}

	if entry.Provider != ProviderOpenAI {
		t.Errorf("LogEntry.Provider = %v, want %v", entry.Provider, ProviderOpenAI)
	}

	if entry.Duration != 100*time.Millisecond {
		t.Errorf("LogEntry.Duration = %v, want 100ms", entry.Duration)
	}

	if entry.IPAddress != "127.0.0.1" {
		t.Errorf("LogEntry.IPAddress = %v, want '127.0.0.1'", entry.IPAddress)
	}
}

func TestChatChunkCreation(t *testing.T) {
	delta := &MessageDelta{
		Role:    RoleAssistant,
		Content: "Hello",
	}

	chunk := ChatChunk{
		ID:    "chatcmpl-123",
		Model: "gpt-4",
		Choices: []Choice{
			{
				Index:        0,
				Delta:        delta,
				FinishReason: "",
			},
		},
	}

	if chunk.ID != "chatcmpl-123" {
		t.Errorf("ChatChunk.ID = %v, want 'chatcmpl-123'", chunk.ID)
	}

	if chunk.Model != "gpt-4" {
		t.Errorf("ChatChunk.Model = %v, want 'gpt-4'", chunk.Model)
	}

	if len(chunk.Choices) != 1 {
		t.Fatalf("ChatChunk.Choices length = %v, want 1", len(chunk.Choices))
	}

	choice := chunk.Choices[0]
	if choice.Index != 0 {
		t.Errorf("Choice.Index = %v, want 0", choice.Index)
	}

	if choice.Delta.Content != "Hello" {
		t.Errorf("Delta.Content = %v, want 'Hello'", choice.Delta.Content)
	}
}

func TestStreamErrorCreation(t *testing.T) {
	err := &StreamError{
		Provider: ProviderAnthropic,
		Message:  "Connection timeout",
		Code:     "timeout",
	}

	if err.Provider != ProviderAnthropic {
		t.Errorf("StreamError.Provider = %v, want %v", err.Provider, ProviderAnthropic)
	}

	if err.Message != "Connection timeout" {
		t.Errorf("StreamError.Message = %v, want 'Connection timeout'", err.Message)
	}

	if err.Code != "timeout" {
		t.Errorf("StreamError.Code = %v, want 'timeout'", err.Code)
	}

	expectedError := "anthropic: Connection timeout (timeout)"
	if err.Error() != expectedError {
		t.Errorf("StreamError.Error() = %v, want %v", err.Error(), expectedError)
	}
}

func TestChoiceCreation(t *testing.T) {
	choice := Choice{
		Index: 0,
		Message: Message{
			Role:    RoleAssistant,
			Content: "Test response",
		},
		FinishReason: "stop",
	}

	if choice.Index != 0 {
		t.Errorf("Choice.Index = %v, want 0", choice.Index)
	}

	if choice.Message.Role != RoleAssistant {
		t.Errorf("Choice.Message.Role = %v, want %v", choice.Message.Role, RoleAssistant)
	}

	if choice.FinishReason != "stop" {
		t.Errorf("Choice.FinishReason = %v, want 'stop'", choice.FinishReason)
	}
}

func TestMessageDeltaCreation(t *testing.T) {
	delta := MessageDelta{
		Role:    RoleAssistant,
		Content: "Hello, world!",
	}

	if delta.Role != RoleAssistant {
		t.Errorf("MessageDelta.Role = %v, want %v", delta.Role, RoleAssistant)
	}

	if delta.Content != "Hello, world!" {
		t.Errorf("MessageDelta.Content = %v, want 'Hello, world!'", delta.Content)
	}
}

func TestUsage(t *testing.T) {
	usage := Usage{
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
	}

	if usage.PromptTokens != 100 {
		t.Errorf("Usage.PromptTokens = %v, want 100", usage.PromptTokens)
	}

	if usage.CompletionTokens != 50 {
		t.Errorf("Usage.CompletionTokens = %v, want 50", usage.CompletionTokens)
	}

	if usage.TotalTokens != 150 {
		t.Errorf("Usage.TotalTokens = %v, want 150", usage.TotalTokens)
	}
}
