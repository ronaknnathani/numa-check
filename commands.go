package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
)

func execStderr(err error) error {
	if exitErr, ok := err.(*exec.ExitError); ok && len(exitErr.Stderr) > 0 {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(exitErr.Stderr)))
	}
	return err
}

func getGPUInfo(cmd CommandRunner) (map[string]string, error) {
	slog.Debug("querying nvidia-smi for GPU info")
	out, err := cmd.Run("nvidia-smi", "--query-gpu=uuid,pci.bus_id", "--format=csv,noheader")
	if err != nil {
		return nil, execStderr(err)
	}
	gpuMap := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		parts := strings.SplitN(line, ",", 2)
		if len(parts) < 2 {
			continue
		}
		uuid := strings.TrimSpace(parts[0])
		pciID := normalizePCI(strings.TrimSpace(parts[1]))
		gpuMap[uuid] = pciID
		slog.Debug("found GPU", "uuid", uuid, "pci", pciID)
	}
	return gpuMap, nil
}

func getContainerInfo(cmd CommandRunner, podName, containerName string) (ContainerInfo, error) {
	slog.Debug("looking up container", "pod", podName, "container", containerName)
	out, err := cmd.Run("crictl", "ps", "-o", "json")
	if err != nil {
		return ContainerInfo{}, fmt.Errorf("failed to run crictl ps: %v", execStderr(err))
	}

	var psOut crictlPSOutput
	if err := json.Unmarshal(out, &psOut); err != nil {
		return ContainerInfo{}, fmt.Errorf("failed to parse crictl ps output: %v", err)
	}

	var targetContainerID string
	for _, ctr := range psOut.Containers {
		if ctr.Metadata.Name == containerName {
			if podLabel, ok := ctr.Labels["io.kubernetes.pod.name"]; ok && podLabel == podName {
				targetContainerID = ctr.ID
				break
			}
		}
	}
	if targetContainerID == "" {
		return ContainerInfo{}, fmt.Errorf("container %q in pod %q not found", containerName, podName)
	}

	inspectOut, err := cmd.Run("crictl", "inspect", "-o", "json", targetContainerID)
	if err != nil {
		return ContainerInfo{}, fmt.Errorf("crictl inspect failed: %v", execStderr(err))
	}

	var inspectData crictlInspectOutput
	if err := json.Unmarshal(inspectOut, &inspectData); err != nil {
		return ContainerInfo{}, fmt.Errorf("failed to parse crictl inspect output: %v", err)
	}

	if inspectData.Info.PID <= 0 {
		return ContainerInfo{}, fmt.Errorf("invalid PID %d in crictl inspect output", inspectData.Info.PID)
	}

	slog.Debug("resolved container", "pid", inspectData.Info.PID,
		"cpu_quota", inspectData.Info.Config.Linux.Resources.CPUQuota,
		"cpu_period", inspectData.Info.Config.Linux.Resources.CPUPeriod,
		"mem_limit", inspectData.Info.Config.Linux.Resources.MemoryLimitInBytes)

	return ContainerInfo{
		PID:       inspectData.Info.PID,
		Resources: inspectData.Info.Config.Linux.Resources,
	}, nil
}

func formatResources(res crictlResources, gpuCount int) string {
	var sb strings.Builder
	if res.CPUShares > 2 {
		cores := float64(res.CPUShares) / 1024
		sb.WriteString(fmt.Sprintf("  CPU request .......... %.1f cores\n", cores))
	}
	if res.CPUQuota > 0 && res.CPUPeriod > 0 {
		cores := float64(res.CPUQuota) / float64(res.CPUPeriod)
		sb.WriteString(fmt.Sprintf("  CPU limit ............ %.1f cores\n", cores))
	}
	if res.MemoryLimitInBytes > 0 {
		sb.WriteString(fmt.Sprintf("  Memory limit ......... %s\n", formatBytes(res.MemoryLimitInBytes)))
	}
	if gpuCount > 0 {
		sb.WriteString(fmt.Sprintf("  GPUs ................. %d\n", gpuCount))
	}
	return sb.String()
}

func formatBytes(b int64) string {
	const (
		gib = 1 << 30
		mib = 1 << 20
	)
	switch {
	case b >= gib:
		return fmt.Sprintf("%.1f GiB", float64(b)/float64(gib))
	case b >= mib:
		return fmt.Sprintf("%.1f MiB", float64(b)/float64(mib))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

func runNumastat(cmd CommandRunner, pid int) (string, error) {
	slog.Debug("running numastat", "pid", pid)
	out, err := cmd.Run("numastat", "-p", fmt.Sprintf("%d", pid))
	if err != nil {
		return "", execStderr(err)
	}
	var sb strings.Builder
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		sb.WriteString("  ")
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	return sb.String(), nil
}
