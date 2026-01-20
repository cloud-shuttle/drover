// Package memory provides memory tracking for worker processes
package memory

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Tracker tracks memory usage of worker processes
type Tracker struct {
	mu            sync.RWMutex
	workers       map[int]*WorkerMemory // PID -> memory info
	samplingRate  time.Duration
	maxSampleSize int
}

// WorkerMemory represents memory metrics for a worker process
type WorkerMemory struct {
	PID         int       `json:"pid"`
	RSSBytes    int64     `json:"rss_bytes"`    // Resident Set Size in bytes
	VMSBytes    int64     `json:"vms_bytes"`    // Virtual Memory Size in bytes
	LastUpdated time.Time `json:"last_updated"`
	SampleCount int       `json:"sample_count"`
	PeakRSS     int64     `json:"peak_rss"` // Peak RSS observed
}

// Stats aggregates memory statistics across all tracked workers
type Stats struct {
	TotalWorkers    int       `json:"total_workers"`
	TotalRSSBytes   int64     `json:"total_rss_bytes"`
	AvgRSSBytes     int64     `json:"avg_rss_bytes"`
	PeakRSSBytes    int64     `json:"peak_rss_bytes"`
	LastUpdated     time.Time `json:"last_updated"`
	SystemTotalMB   int64     `json:"system_total_mb"`
	SystemAvailableMB int64   `json:"system_available_mb"`
	SystemUsedPercent float64 `json:"system_used_percent"`
}

// NewTracker creates a new memory tracker
func NewTracker() *Tracker {
	return &Tracker{
		workers:       make(map[int]*WorkerMemory),
		samplingRate:  5 * time.Second,
		maxSampleSize: 100,
	}
}

// Track adds a worker process to be tracked
func (t *Tracker) Track(pid int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.workers[pid] = &WorkerMemory{
		PID:         pid,
		LastUpdated: time.Now(),
	}
}

// Untrack removes a worker process from tracking
func (t *Tracker) Untrack(pid int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.workers, pid)
}

// Sample samples memory usage for all tracked workers
func (t *Tracker) Sample() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	var pids []int
	for pid := range t.workers {
		pids = append(pids, pid)
	}

	// Sample each worker
	for _, pid := range pids {
		mem, err := GetProcessMemory(pid)
		if err != nil {
			// Process may have exited, remove from tracking
			delete(t.workers, pid)
			continue
		}

		w := t.workers[pid]
		w.RSSBytes = mem.RSSBytes
		w.VMSBytes = mem.VMSBytes
		w.LastUpdated = time.Now()
		w.SampleCount++

		// Track peak RSS
		if mem.RSSBytes > w.PeakRSS {
			w.PeakRSS = mem.RSSBytes
		}
	}

	return nil
}

// GetStats returns aggregated memory statistics
func (t *Tracker) GetStats() *Stats {
	t.mu.RLock()
	defer t.mu.RUnlock()

	stats := &Stats{
		TotalWorkers: len(t.workers),
		LastUpdated:  time.Now(),
	}

	if len(t.workers) == 0 {
		return stats
	}

	var totalRSS int64
	var peakRSS int64

	for _, w := range t.workers {
		totalRSS += w.RSSBytes
		if w.PeakRSS > peakRSS {
			peakRSS = w.PeakRSS
		}
	}

	stats.TotalRSSBytes = totalRSS
	stats.AvgRSSBytes = totalRSS / int64(len(t.workers))
	stats.PeakRSSBytes = peakRSS

	// Get system memory info
	if sysMem, err := GetSystemMemory(); err == nil {
		stats.SystemTotalMB = sysMem.TotalMB
		stats.SystemAvailableMB = sysMem.AvailableMB
		stats.SystemUsedPercent = sysMem.UsedPercent
	}

	return stats
}

// GetWorkerMemory returns memory info for a specific worker
func (t *Tracker) GetWorkerMemory(pid int) (*WorkerMemory, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	w, ok := t.workers[pid]
	return w, ok
}

// ShouldThrottle returns true if memory usage indicates we should throttle
func (t *Tracker) ShouldThrottle(thresholdMB int64) bool {
	stats := t.GetStats()
	if stats.SystemAvailableMB > 0 && stats.SystemAvailableMB < thresholdMB {
		return true
	}
	return false
}

// Start starts the background sampling goroutine
func (t *Tracker) Start() {
	go func() {
		ticker := time.NewTicker(t.samplingRate)
		defer ticker.Stop()

		for range ticker.C {
			_ = t.Sample()
		}
	}()
}

// GetProcessMemory retrieves memory usage for a process by PID
func GetProcessMemory(pid int) (*WorkerMemory, error) {
	// On Linux, read from /proc/[pid]/statm
	statmPath := filepath.Join("/proc", strconv.Itoa(pid), "statm")
	data, err := os.ReadFile(statmPath)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", statmPath, err)
	}

	fields := strings.Fields(string(data))
	if len(fields) < 2 {
		return nil, fmt.Errorf("invalid statm format")
	}

	// statm format: rss pages, vms pages (in pagesize)
	// We need to get the page size
	pageSize := int64(os.Getpagesize())

	rssPages, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parsing rss: %w", err)
	}

	vmsPages, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parsing vms: %w", err)
	}

	return &WorkerMemory{
		PID:      pid,
		RSSBytes: rssPages * pageSize,
		VMSBytes: vmsPages * pageSize,
	}, nil
}

// SystemMemory represents system-wide memory information
type SystemMemory struct {
	TotalMB       int64   `json:"total_mb"`
	AvailableMB   int64   `json:"available_mb"`
	UsedPercent   float64 `json:"used_percent"`
}

// GetSystemMemory retrieves system-wide memory information
func GetSystemMemory() (*SystemMemory, error) {
	// Read /proc/meminfo on Linux
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return nil, fmt.Errorf("reading /proc/meminfo: %w", err)
	}

	lines := strings.Split(string(data), "\n")
	meminfo := make(map[string]int64)

	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		key := strings.TrimSuffix(fields[0], ":")
		value, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			continue
		}
		meminfo[key] = value
	}

	// Get values in KB, convert to MB
	totalKB := meminfo["MemTotal"]
	availableKB := meminfo["MemAvailable"]
	if availableKB == 0 {
		// Fallback for older kernels: MemAvailable = MemFree + Buffers + Cached
		availableKB = meminfo["MemFree"] + meminfo["Buffers"] + meminfo["Cached"]
	}

	totalMB := totalKB / 1024
	availableMB := availableKB / 1024
	usedPercent := float64(totalKB-availableKB) / float64(totalKB) * 100

	return &SystemMemory{
		TotalMB:     totalMB,
		AvailableMB: availableMB,
		UsedPercent: usedPercent,
	}, nil
}

// GetPID returns the PID of the current process
func GetPID() int {
	return os.Getpid()
}

// GetSelfMemory returns memory usage of the current process
func GetSelfMemory() (*WorkerMemory, error) {
	return GetProcessMemory(GetPID())
}

// GetParentPID returns the PID of the parent process
func GetParentPID() int {
	return os.Getppid()
}

// GetOrFindWorkerPID attempts to find a worker PID by searching processes
// This is a fallback when the worker PID is not directly known
func GetOrFindWorkerPID(workerBinary string) (int, error) {
	// Use pgrep to find the worker process
	cmd := exec.Command("pgrep", "-n", workerBinary)
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("pgrep failed: %w", err)
	}

	pidStr := strings.TrimSpace(string(output))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return 0, fmt.Errorf("parsing pid: %w", err)
	}

	return pid, nil
}

// FormatBytes formats a byte count as a human-readable string
func FormatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
