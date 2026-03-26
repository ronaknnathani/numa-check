//go:build !linux

package main

import "fmt"

func getCPUAffinity(pid int) ([]int, error) {
	return nil, fmt.Errorf("CPU affinity is only supported on Linux")
}
