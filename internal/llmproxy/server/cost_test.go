package server

import (
	"context"
	"testing"
	"time"

	"github.com/cloud-shuttle/drover/internal/llmproxy"
)

func TestNewCostTracker(t *testing.T) {
	cfg := llmproxy.CostBudgetConfig{
		HourlyLimit: 10.0,
		DailyLimit:  100.0,
	}

	tracker := NewCostTracker(cfg)

	if tracker == nil {
		t.Fatal("NewCostTracker() returned nil")
	}

	if tracker.config.HourlyLimit != 10.0 {
		t.Errorf("CostTracker.config.HourlyLimit = %v, want 10.0", tracker.config.HourlyLimit)
	}
}

func TestCostTrackerRecordCost(t *testing.T) {
	cfg := llmproxy.CostBudgetConfig{
		HourlyLimit: 10.0,
		DailyLimit:  100.0,
		Enabled:     true,
	}

	tracker := NewCostTracker(cfg)

	cost := &llmproxy.Cost{
		Provider:  llmproxy.ProviderOpenAI,
		Model:     "gpt-4",
		InputCost:  0.01,
		OutputCost: 0.02,
		TotalCost:  0.03,
		Timestamp: time.Now(),
	}

	tracker.RecordCost(cost)

	if tracker.hourlyCost != 0.03 {
		t.Errorf("CostTracker.hourlyCost = %v, want 0.03", tracker.hourlyCost)
	}

	if tracker.dailyCost != 0.03 {
		t.Errorf("CostTracker.dailyCost = %v, want 0.03", tracker.dailyCost)
	}
}

func TestCostTrackerCheckBudget(t *testing.T) {
	cfg := llmproxy.CostBudgetConfig{
		HourlyLimit: 0.05,
		DailyLimit:  1.0,
		Enabled:     true,
	}

	tracker := NewCostTracker(cfg)

	// First cost should be within budget
	cost1 := &llmproxy.Cost{
		TotalCost: 0.03,
		Timestamp: time.Now(),
	}

	tracker.RecordCost(cost1)

	allowed := tracker.CheckBudget(context.Background())
	if !allowed {
		t.Error("CheckBudget() should return true when under budget")
	}

	// Second cost should exceed hourly budget
	cost2 := &llmproxy.Cost{
		TotalCost: 0.03,
		Timestamp: time.Now(),
	}

	tracker.RecordCost(cost2)

	allowed = tracker.CheckBudget(context.Background())
	if allowed {
		t.Error("CheckBudget() should return false when over hourly budget")
	}
}

func TestCostTrackerGetStats(t *testing.T) {
	cfg := llmproxy.CostBudgetConfig{
		HourlyLimit: 10.0,
		DailyLimit:  100.0,
		Enabled:     true,
	}

	tracker := NewCostTracker(cfg)

	cost := &llmproxy.Cost{
		TotalCost: 1.5,
		Timestamp: time.Now(),
	}

	tracker.RecordCost(cost)

	stats := tracker.GetStats()
	if stats == nil {
		t.Fatal("GetStats() returned nil")
	}

	hourly := stats["cost_hourly"].(float64)
	if hourly != 1.5 {
		t.Errorf("GetStats()[cost_hourly] = %v, want 1.5", hourly)
	}

	daily := stats["cost_daily"].(float64)
	if daily != 1.5 {
		t.Errorf("GetStats()[cost_daily] = %v, want 1.5", daily)
	}
}

func TestCostTrackerGetStatsDaily(t *testing.T) {
	cfg := llmproxy.CostBudgetConfig{
		HourlyLimit: 10.0,
		DailyLimit:  100.0,
		Enabled:     true,
	}

	tracker := NewCostTracker(cfg)

	cost := &llmproxy.Cost{
		TotalCost: 2.5,
		Timestamp: time.Now(),
	}

	tracker.RecordCost(cost)

	stats := tracker.GetStats()
	daily := stats["cost_daily"].(float64)
	if daily != 2.5 {
		t.Errorf("GetStats()[cost_daily] = %v, want 2.5", daily)
	}
}

func TestCostTrackerResetHourly(t *testing.T) {
	cfg := llmproxy.CostBudgetConfig{
		HourlyLimit: 10.0,
	}

	tracker := NewCostTracker(cfg)

	// Record some cost
	cost := &llmproxy.Cost{
		TotalCost: 5.0,
		Timestamp: time.Now(),
	}

	tracker.RecordCost(cost)

	// Manually set hourly reset time to past
	tracker.hourlyReset = time.Now().Add(-1 * time.Hour)

	// Trigger reset by checking budget
	tracker.CheckBudget(context.Background())

	if tracker.hourlyCost != 0 {
		t.Errorf("After hourly reset, hourlyCost = %v, want 0", tracker.hourlyCost)
	}
}

func TestCostTrackerResetDaily(t *testing.T) {
	cfg := llmproxy.CostBudgetConfig{
		DailyLimit: 100.0,
	}

	tracker := NewCostTracker(cfg)

	// Record some cost
	cost := &llmproxy.Cost{
		TotalCost: 50.0,
		Timestamp: time.Now(),
	}

	tracker.RecordCost(cost)

	// Manually set daily reset time to past
	tracker.dailyReset = time.Now().Add(-25 * time.Hour)

	// Trigger reset by checking budget
	tracker.CheckBudget(context.Background())

	if tracker.dailyCost != 0 {
		t.Errorf("After daily reset, dailyCost = %v, want 0", tracker.dailyCost)
	}
}

func TestCostTrackerNoBudgetConfig(t *testing.T) {
	cfg := llmproxy.CostBudgetConfig{
		HourlyLimit: 0,
		DailyLimit:  0,
	}

	tracker := NewCostTracker(cfg)

	cost := &llmproxy.Cost{
		TotalCost: 1000.0,
		Timestamp: time.Now(),
	}

	tracker.RecordCost(cost)

	// With no budget limits, should always be allowed
	allowed := tracker.CheckBudget(context.Background())
	if !allowed {
		t.Error("CheckBudget() should return true when no budget is configured")
	}
}

func TestCostTrackerMultipleCosts(t *testing.T) {
	cfg := llmproxy.CostBudgetConfig{
		HourlyLimit: 10.0,
		Enabled:     true,
	}

	tracker := NewCostTracker(cfg)

	costs := []*llmproxy.Cost{
		{TotalCost: 1.0, Timestamp: time.Now()},
		{TotalCost: 2.0, Timestamp: time.Now()},
		{TotalCost: 3.0, Timestamp: time.Now()},
	}

	for _, cost := range costs {
		tracker.RecordCost(cost)
	}

	if tracker.hourlyCost != 6.0 {
		t.Errorf("After recording multiple costs, hourlyCost = %v, want 6.0", tracker.hourlyCost)
	}
}
