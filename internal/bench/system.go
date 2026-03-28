package bench

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func collectHostSnapshot(goBinary, pythonBinary string) HostSnapshot {
	return HostSnapshot{
		OS:            runtime.GOOS,
		Architecture:  runtime.GOARCH,
		Kernel:        kernelVersion(),
		CPUModel:      cpuModel(),
		LogicalCores:  runtime.NumCPU(),
		TotalRAMBytes: totalRAMBytes(),
		GoVersion:     commandVersion(goBinary, []string{"version"}),
		PythonVersion: commandVersion(pythonBinary, []string{"--version"}),
	}
}

func commandVersion(binary string, args []string) string {
	if binary == "" {
		return ""
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return ""
	}

	output := strings.TrimSpace(stdout.String())
	if output == "" {
		output = strings.TrimSpace(stderr.String())
	}
	return output
}

func gitRevision(repoPath string) string {
	if repoPath == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "rev-parse", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func sha256File(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func cpuModel() string {
	file, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "model name") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return ""
}

func totalRAMBytes() uint64 {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				value, parseErr := strconv.ParseUint(fields[1], 10, 64)
				if parseErr == nil {
					return value * 1024
				}
			}
		}
	}
	return 0
}

func kernelVersion() string {
	var uts syscall.Utsname
	if err := syscall.Uname(&uts); err != nil {
		return ""
	}
	return utsToString(uts.Release)
}

func utsToString(values [65]int8) string {
	result := make([]byte, 0, len(values))
	for _, value := range values {
		if value == 0 {
			break
		}
		result = append(result, byte(value))
	}
	return string(result)
}
