//go:build linux

package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// GetProcessMemory retrieves memory usage for a process by PID using /proc/[pid]/statm.
func GetProcessMemory(pid int) (*WorkerMemory, error) {
	statmPath := filepath.Join("/proc", strconv.Itoa(pid), "statm")
	data, err := os.ReadFile(statmPath)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", statmPath, err)
	}

	fields := strings.Fields(string(data))
	if len(fields) < 2 {
		return nil, fmt.Errorf("invalid statm format")
	}

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

// GetSystemMemory retrieves system-wide memory information from /proc/meminfo.
func GetSystemMemory() (*SystemMemory, error) {
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

	totalKB := meminfo["MemTotal"]
	availableKB := meminfo["MemAvailable"]
	if availableKB == 0 {
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
