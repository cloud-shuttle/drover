//go:build darwin

package memory

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// GetProcessMemory retrieves memory usage for a process by PID using ps on macOS.
func GetProcessMemory(pid int) (*WorkerMemory, error) {
	// Use ps to get RSS and VSZ for the process (in KB)
	cmd := exec.Command("ps", "-o", "rss=,vsz=", "-p", strconv.Itoa(pid))
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ps failed for pid %d: %w", pid, err)
	}

	fields := strings.Fields(strings.TrimSpace(string(output)))
	if len(fields) < 2 {
		return nil, fmt.Errorf("unexpected ps output: %q", string(output))
	}

	rssKB, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parsing rss: %w", err)
	}

	vszKB, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parsing vsz: %w", err)
	}

	return &WorkerMemory{
		PID:      pid,
		RSSBytes: rssKB * 1024,
		VMSBytes: vszKB * 1024,
	}, nil
}

// GetSystemMemory retrieves system-wide memory information using sysctl on macOS.
func GetSystemMemory() (*SystemMemory, error) {
	// Get total physical memory
	totalOutput, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
	if err != nil {
		return nil, fmt.Errorf("sysctl hw.memsize: %w", err)
	}

	totalBytes, err := strconv.ParseInt(strings.TrimSpace(string(totalOutput)), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parsing total memory: %w", err)
	}
	totalMB := totalBytes / (1024 * 1024)

	// Get page size and vm_stat for available memory estimation
	pageSizeOutput, err := exec.Command("sysctl", "-n", "hw.pagesize").Output()
	if err != nil {
		return nil, fmt.Errorf("sysctl hw.pagesize: %w", err)
	}
	pageSize, err := strconv.ParseInt(strings.TrimSpace(string(pageSizeOutput)), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parsing page size: %w", err)
	}

	vmStatOutput, err := exec.Command("vm_stat").Output()
	if err != nil {
		return nil, fmt.Errorf("vm_stat: %w", err)
	}

	// Parse vm_stat output to estimate free/available memory
	var freePages, inactivePages int64
	for _, line := range strings.Split(string(vmStatOutput), "\n") {
		if strings.HasPrefix(line, "Pages free:") {
			freePages = parseVMStatValue(line)
		} else if strings.HasPrefix(line, "Pages inactive:") {
			inactivePages = parseVMStatValue(line)
		}
	}

	availableMB := (freePages + inactivePages) * pageSize / (1024 * 1024)
	var usedPercent float64
	if totalMB > 0 {
		usedPercent = float64(totalMB-availableMB) / float64(totalMB) * 100
	}

	return &SystemMemory{
		TotalMB:     totalMB,
		AvailableMB: availableMB,
		UsedPercent: usedPercent,
	}, nil
}

// parseVMStatValue extracts the numeric value from a vm_stat output line.
func parseVMStatValue(line string) int64 {
	parts := strings.Split(line, ":")
	if len(parts) < 2 {
		return 0
	}
	numStr := strings.TrimSpace(strings.TrimSuffix(parts[1], "."))
	val, _ := strconv.ParseInt(numStr, 10, 64)
	return val
}
