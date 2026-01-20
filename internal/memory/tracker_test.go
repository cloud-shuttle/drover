// Package memory provides tests for memory tracking functionality
package memory

import (
	"os"
	"testing"
	"time"
)

func TestGetPID(t *testing.T) {
	pid := GetPID()
	if pid <= 0 {
		t.Errorf("GetPID() returned invalid PID: %d", pid)
	}
}

func TestGetParentPID(t *testing.T) {
	ppid := GetParentPID()
	if ppid <= 0 {
		t.Errorf("GetParentPID() returned invalid PID: %d", ppid)
	}
}

func TestGetSelfMemory(t *testing.T) {
	mem, err := GetSelfMemory()
	if err != nil {
		t.Fatalf("GetSelfMemory() failed: %v", err)
	}

	if mem.PID <= 0 {
		t.Errorf("GetSelfMemory() returned invalid PID: %d", mem.PID)
	}

	if mem.RSSBytes <= 0 {
		t.Errorf("GetSelfMemory() returned invalid RSS: %d", mem.RSSBytes)
	}

	if mem.VMSBytes <= 0 {
		t.Errorf("GetSelfMemory() returned invalid VMS: %d", mem.VMSBytes)
	}
}

func TestGetProcessMemory(t *testing.T) {
	// Test getting memory for the current process
	pid := GetPID()
	mem, err := GetProcessMemory(pid)
	if err != nil {
		t.Fatalf("GetProcessMemory(%d) failed: %v", pid, err)
	}

	if mem.PID != pid {
		t.Errorf("GetProcessMemory(%d) returned wrong PID: %d", pid, mem.PID)
	}

	if mem.RSSBytes <= 0 {
		t.Errorf("GetProcessMemory(%d) returned invalid RSS: %d", pid, mem.RSSBytes)
	}

	if mem.VMSBytes <= 0 {
		t.Errorf("GetProcessMemory(%d) returned invalid VMS: %d", pid, mem.VMSBytes)
	}
}

func TestGetProcessMemoryInvalidPID(t *testing.T) {
	// Test with an invalid PID
	_, err := GetProcessMemory(999999999)
	if err == nil {
		t.Error("GetProcessMemory(999999999) should have failed but didn't")
	}
}

func TestGetSystemMemory(t *testing.T) {
	mem, err := GetSystemMemory()
	if err != nil {
		t.Fatalf("GetSystemMemory() failed: %v", err)
	}

	if mem.TotalMB <= 0 {
		t.Errorf("GetSystemMemory() returned invalid TotalMB: %d", mem.TotalMB)
	}

	if mem.AvailableMB < 0 {
		t.Errorf("GetSystemMemory() returned invalid AvailableMB: %d", mem.AvailableMB)
	}

	if mem.AvailableMB > mem.TotalMB {
		t.Errorf("GetSystemMemory() returned AvailableMB > TotalMB: %d > %d",
			mem.AvailableMB, mem.TotalMB)
	}

	if mem.UsedPercent < 0 || mem.UsedPercent > 100 {
		t.Errorf("GetSystemMemory() returned invalid UsedPercent: %f", mem.UsedPercent)
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes    int64
		expected string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1024 * 1024, "1.0 MB"},
		{1536 * 1024, "1.5 MB"},
		{1024 * 1024 * 1024, "1.0 GB"},
		{1536 * 1024 * 1024, "1.5 GB"},
		{1024 * 1024 * 1024 * 1024, "1.0 TB"},
	}

	for _, tt := range tests {
		result := FormatBytes(tt.bytes)
		if result != tt.expected {
			t.Errorf("FormatBytes(%d) = %s, want %s", tt.bytes, result, tt.expected)
		}
	}
}

func TestNewTracker(t *testing.T) {
	tracker := NewTracker()
	if tracker == nil {
		t.Fatal("NewTracker() returned nil")
	}

	if tracker.workers == nil {
		t.Error("NewTracker() workers map is nil")
	}

	if tracker.samplingRate != 5*time.Second {
		t.Errorf("NewTracker() samplingRate = %v, want %v", tracker.samplingRate, 5*time.Second)
	}

	if tracker.maxSampleSize != 100 {
		t.Errorf("NewTracker() maxSampleSize = %d, want %d", tracker.maxSampleSize, 100)
	}
}

func TestTrackerTrackUntrack(t *testing.T) {
	tracker := NewTracker()

	// Track a PID
	pid := GetPID()
	tracker.Track(pid)

	// Verify it's tracked
	w, ok := tracker.GetWorkerMemory(pid)
	if !ok {
		t.Error("Track() didn't track the PID")
	}

	if w.PID != pid {
		t.Errorf("Track() stored wrong PID: %d, want %d", w.PID, pid)
	}

	// Untrack the PID
	tracker.Untrack(pid)

	// Verify it's untracked
	_, ok = tracker.GetWorkerMemory(pid)
	if ok {
		t.Error("Untrack() didn't untrack the PID")
	}
}

func TestTrackerGetStats(t *testing.T) {
	tracker := NewTracker()

	// Empty tracker should return zero stats
	stats := tracker.GetStats()
	if stats.TotalWorkers != 0 {
		t.Errorf("GetStats() TotalWorkers = %d, want 0", stats.TotalWorkers)
	}

	// Track some PIDs
	pid1 := GetPID()
	pid2 := os.Getppid()

	tracker.Track(pid1)
	tracker.Track(pid2)

	// Sample to populate memory data
	tracker.Sample()

	// Check stats
	stats = tracker.GetStats()
	if stats.TotalWorkers != 2 {
		t.Errorf("GetStats() TotalWorkers = %d, want 2", stats.TotalWorkers)
	}

	if stats.TotalRSSBytes <= 0 {
		t.Errorf("GetStats() TotalRSSBytes = %d, want > 0", stats.TotalRSSBytes)
	}

	if stats.AvgRSSBytes <= 0 {
		t.Errorf("GetStats() AvgRSSBytes = %d, want > 0", stats.AvgRSSBytes)
	}
}

func TestTrackerShouldThrottle(t *testing.T) {
	tracker := NewTracker()

	// Get system memory to set a reasonable threshold
	sysMem, err := GetSystemMemory()
	if err != nil {
		t.Skipf("Skipping throttle test: GetSystemMemory() failed: %v", err)
	}

	// Set threshold very high - should not throttle
	highThreshold := sysMem.TotalMB * 2
	if tracker.ShouldThrottle(highThreshold) {
		t.Error("ShouldThrottle() returned true with very high threshold")
	}

	// Set threshold very low - should throttle if we can read memory
	lowThreshold := int64(1)
	result := tracker.ShouldThrottle(lowThreshold)
	// We can't assert the result here because it depends on system state
	// Just verify the function doesn't panic
	_ = result
}

func TestTrackerSample(t *testing.T) {
	tracker := NewTracker()

	// Track current process
	pid := GetPID()
	tracker.Track(pid)

	// Sample should succeed
	err := tracker.Sample()
	if err != nil {
		t.Errorf("Sample() failed: %v", err)
	}

	// Verify memory data was populated
	w, ok := tracker.GetWorkerMemory(pid)
	if !ok {
		t.Fatal("Sample() didn't keep tracked PID")
	}

	if w.RSSBytes <= 0 {
		t.Error("Sample() didn't populate RSS")
	}

	if w.VMSBytes <= 0 {
		t.Error("Sample() didn't populate VMS")
	}

	if w.SampleCount != 1 {
		t.Errorf("Sample() SampleCount = %d, want 1", w.SampleCount)
	}
}

func TestTrackerSampleRemovesExitedProcesses(t *testing.T) {
	tracker := NewTracker()

	// Track an invalid PID
	tracker.Track(999999999)

	// Sample should remove the invalid PID
	tracker.Sample()

	// Verify it was removed
	_, ok := tracker.GetWorkerMemory(999999999)
	if ok {
		t.Error("Sample() didn't remove invalid PID")
	}
}

func TestTrackerTrackPeakRSS(t *testing.T) {
	tracker := NewTracker()

	// Track current process
	pid := GetPID()
	tracker.Track(pid)

	// Sample multiple times to track peak
	var initialPeak int64
	for i := 0; i < 3; i++ {
		tracker.Sample()
		w, _ := tracker.GetWorkerMemory(pid)

		if i == 0 {
			initialPeak = w.PeakRSS
		} else {
			// Peak should never decrease
			if w.PeakRSS < initialPeak {
				t.Errorf("PeakRSS decreased from %d to %d", initialPeak, w.PeakRSS)
			}
		}

		time.Sleep(10 * time.Millisecond)
	}
}
