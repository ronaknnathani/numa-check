//go:build linux

package main

import "golang.org/x/sys/unix"

func getCPUAffinity(pid int) ([]int, error) {
	var set unix.CPUSet
	if err := unix.SchedGetaffinity(pid, &set); err != nil {
		return nil, err
	}
	var cpus []int
	for i := 0; i < 1024; i++ {
		if set.IsSet(i) {
			cpus = append(cpus, i)
		}
	}
	return cpus, nil
}
