package main

import (
	"fmt"
	"strconv"
	"strings"
)

// expandCPUList parses a CPU list string like "0-3,8-11" into a sorted slice of CPU IDs.
func expandCPUList(s string) ([]int, error) {
	if s == "" {
		return nil, fmt.Errorf("empty CPU list")
	}
	var cpus []int
	for _, token := range strings.Split(strings.ReplaceAll(s, " ", ","), ",") {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		if strings.Contains(token, "-") {
			parts := strings.SplitN(token, "-", 2)
			start, err := strconv.Atoi(parts[0])
			if err != nil {
				return nil, fmt.Errorf("invalid CPU range %q: %v", token, err)
			}
			end, err := strconv.Atoi(parts[1])
			if err != nil {
				return nil, fmt.Errorf("invalid CPU range %q: %v", token, err)
			}
			if start > end {
				return nil, fmt.Errorf("invalid CPU range %q: start > end", token)
			}
			for i := start; i <= end; i++ {
				cpus = append(cpus, i)
			}
		} else {
			n, err := strconv.Atoi(token)
			if err != nil {
				return nil, fmt.Errorf("invalid CPU number %q: %v", token, err)
			}
			cpus = append(cpus, n)
		}
	}
	if len(cpus) == 0 {
		return nil, fmt.Errorf("empty CPU list after parsing")
	}
	return cpus, nil
}

// normalizePCI normalizes a PCI bus ID to the standard 4-digit domain format.
// nvidia-smi sometimes returns 8-digit domain prefixes (e.g., "00000000:3B:00.0").
func normalizePCI(pciID string) string {
	normalized := strings.ToLower(strings.TrimSpace(pciID))
	if strings.HasPrefix(normalized, "00000000:") {
		normalized = "0000:" + normalized[len("00000000:"):]
	}
	return normalized
}
