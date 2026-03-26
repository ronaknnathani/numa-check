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

func getPIDFromContainer(cmd CommandRunner, podName, containerName string) (int, error) {
	slog.Debug("looking up container PID", "pod", podName, "container", containerName)
	out, err := cmd.Run("crictl", "ps", "-o", "json")
	if err != nil {
		return 0, fmt.Errorf("failed to run crictl ps: %v", execStderr(err))
	}

	var psOut crictlPSOutput
	if err := json.Unmarshal(out, &psOut); err != nil {
		return 0, fmt.Errorf("failed to parse crictl ps output: %v", err)
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
		return 0, fmt.Errorf("container %q in pod %q not found", containerName, podName)
	}

	inspectOut, err := cmd.Run("crictl", "inspect", "-o", "json", targetContainerID)
	if err != nil {
		return 0, fmt.Errorf("crictl inspect failed: %v", execStderr(err))
	}

	var inspectData crictlInspectOutput
	if err := json.Unmarshal(inspectOut, &inspectData); err != nil {
		return 0, fmt.Errorf("failed to parse crictl inspect output: %v", err)
	}

	if inspectData.Info.PID <= 0 {
		return 0, fmt.Errorf("invalid PID %d in crictl inspect output", inspectData.Info.PID)
	}

	slog.Debug("resolved container PID", "pid", inspectData.Info.PID)
	return inspectData.Info.PID, nil
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
