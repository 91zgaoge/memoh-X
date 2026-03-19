package containerd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	hostResolvConf    = "/etc/resolv.conf"
	systemdResolvConf = "/run/systemd/resolve/resolv.conf"
	fallbackResolv    = "nameserver 1.1.1.1\nnameserver 8.8.8.8\n"
)

// ResolveConfSource returns a host path to mount as /etc/resolv.conf.
// Priority order:
// 1. systemd-resolved upstream config (real DNS servers, not stub resolver)
// 2. Host's /etc/resolv.conf filtered to remove Docker internal DNS (127.0.0.11)
// 3. Fallback DNS servers (1.1.1.1 and 8.8.8.8)
func ResolveConfSource(dataDir string) (string, error) {
	if strings.TrimSpace(dataDir) == "" {
		return "", ErrInvalidArgument
	}

	// Priority 1: Try systemd-resolved upstream config
	// /run/systemd/resolve/resolv.conf contains the real upstream DNS servers
	// (e.g., 8.8.8.8, 114.114.114.114), not the stub resolver (127.0.0.53).
	// This is the best source for bot containers that can't access localhost.
	if runtime.GOOS == "darwin" {
		if ok, err := limaFileExists(systemdResolvConf); err != nil {
			return "", err
		} else if ok {
			// Copy to dataDir to make it accessible for bind mount
			if copied, err := copyResolvConf(systemdResolvConf, dataDir); err == nil {
				return copied, nil
			}
		}
	} else if _, err := os.Stat(systemdResolvConf); err == nil {
		// Copy to dataDir to make it accessible for bind mount
		if copied, err := copyResolvConf(systemdResolvConf, dataDir); err == nil {
			return copied, nil
		}
	}

	// Priority 2: Use host's /etc/resolv.conf, but filter out Docker internal DNS
	// When running inside a Docker container, /etc/resolv.conf points to 127.0.0.11
	// which is Docker's embedded DNS. This doesn't work for bot containers using
	// CNI networking (different network namespace), so we need to extract the real
	// upstream DNS servers.
	if _, err := os.Stat(hostResolvConf); err == nil {
		if filtered, err := filterDockerDNS(hostResolvConf, dataDir); err == nil && filtered != "" {
			return filtered, nil
		}
	}

	// Priority 3: Create fallback resolv.conf
	return createFallbackResolv(dataDir)
}

// copyResolvConf copies a resolv.conf file to dataDir, filtering out localhost and private IP addresses.
// This is needed because systemd-resolved's resolv.conf might be on a different filesystem
// or contain addresses that won't work for bot containers (localhost or private IPs not reachable from CNI network).
func copyResolvConf(srcPath, dataDir string) (string, error) {
	content, err := os.ReadFile(srcPath)
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return "", err
	}

	// Filter out localhost and private IP addresses (not reachable from CNI network)
	var filteredLines []string
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Skip localhost and private IP nameservers
		if strings.HasPrefix(line, "nameserver ") {
			ns := strings.TrimPrefix(line, "nameserver ")
			ns = strings.TrimSpace(ns)
			// Skip localhost addresses
			if strings.HasPrefix(ns, "127.") || ns == "::1" {
				continue
			}
			// Skip private IP ranges (not reachable from CNI network)
			if isPrivateIP(ns) {
				continue
			}
		}
		filteredLines = append(filteredLines, line)
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	destPath := filepath.Join(dataDir, "resolv.conf")
	filteredContent := strings.Join(filteredLines, "\n") + "\n"
	if err := os.WriteFile(destPath, []byte(filteredContent), 0o644); err != nil {
		return "", err
	}

	return destPath, nil
}

// filterDockerDNS reads a resolv.conf file and filters out Docker's internal DNS (127.0.0.11).
// If real DNS servers are found in comments (e.g., "# ExtServers: [host(127.0.0.53)]"),
// it extracts and uses them. Otherwise, it copies non-Docker nameservers.
// Returns the path to the filtered resolv.conf file.
func filterDockerDNS(resolvPath, dataDir string) (string, error) {
	file, err := os.Open(resolvPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	var nameservers []string
	var otherLines []string
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Extract real DNS from Docker's comment: "# ExtServers: [host(127.0.0.53)]"
		if strings.Contains(line, "# ExtServers:") {
			if servers := extractExtServers(line); len(servers) > 0 {
				nameservers = append(nameservers, servers...)
				continue
			}
		}

		// Skip Docker internal DNS
		if strings.HasPrefix(line, "nameserver 127.0.0.11") {
			continue
		}

		// Collect real nameservers
		if strings.HasPrefix(line, "nameserver ") {
			ns := strings.TrimPrefix(line, "nameserver ")
			ns = strings.TrimSpace(ns)
			// Skip localhost addresses (Docker internal DNS)
			if strings.HasPrefix(ns, "127.") || ns == "::1" {
				continue
			}
			// Skip private IP addresses (not reachable from CNI network)
			if isPrivateIP(ns) {
				continue
			}
			nameservers = append(nameservers, ns)
			continue
		}

		// Keep other lines (search, options, etc.)
		if line != "" && !strings.HasPrefix(line, "#") {
			otherLines = append(otherLines, line)
		}
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	// If no real nameservers found, return empty to trigger fallback
	if len(nameservers) == 0 {
		return "", fmt.Errorf("no real DNS servers found")
	}

	// Write filtered resolv.conf
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return "", err
	}

	filteredPath := filepath.Join(dataDir, "resolv.conf")
	var content strings.Builder
	content.WriteString("# Filtered resolv.conf (Docker internal DNS removed)\n")
	for _, ns := range nameservers {
		content.WriteString(fmt.Sprintf("nameserver %s\n", ns))
	}
	for _, line := range otherLines {
		content.WriteString(line + "\n")
	}

	if err := os.WriteFile(filteredPath, []byte(content.String()), 0o644); err != nil {
		return "", err
	}

	return filteredPath, nil
}

// extractExtServers parses Docker's ExtServers comment to extract real DNS servers.
// Example: "# ExtServers: [host(127.0.0.53)]" -> ["127.0.0.53"]
// Example: "# ExtServers: [host(10.10.10.10) host(223.5.5.5)]" -> ["223.5.5.5"] (10.10.10.10 filtered as private)
// Filters out private IPs that are not reachable from CNI networks.
func extractExtServers(line string) []string {
	var servers []string
	// Find content between [ and ]
	start := strings.Index(line, "[")
	end := strings.Index(line, "]")
	if start == -1 || end == -1 || start >= end {
		return servers
	}

	content := line[start+1 : end]
	// Split by space or comma and extract host(...) entries
	// Docker uses space-separated: [host(10.10.10.10) host(223.5.5.5)]
	parts := strings.FieldsFunc(content, func(r rune) bool {
		return r == ' ' || r == ','
	})
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "host(") && strings.HasSuffix(part, ")") {
			server := part[5 : len(part)-1] // Extract content between host( and )
			server = strings.TrimSpace(server)
			// Skip empty, localhost, and private IP addresses
			if server != "" && !strings.HasPrefix(server, "127.") && !isPrivateIP(server) {
				servers = append(servers, server)
			}
		}
	}

	return servers
}

// createFallbackResolv creates a fallback resolv.conf with public DNS servers.
func createFallbackResolv(dataDir string) (string, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return "", err
	}
	fallbackPath := filepath.Join(dataDir, "resolv.conf")
	if _, err := os.Stat(fallbackPath); err == nil {
		return fallbackPath, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err := os.WriteFile(fallbackPath, []byte(fallbackResolv), 0o644); err != nil {
		return "", err
	}
	return fallbackPath, nil
}

// isPrivateIP checks if an IP address is in a private range.
// Private IPs are not reachable from CNI networks, so we filter them out
// and use public DNS servers instead.
func isPrivateIP(ip string) bool {
	// IPv4 private ranges
	privatePrefixes := []string{
		"10.",     // 10.0.0.0/8
		"172.16.", // 172.16.0.0/12 - note: covers 172.16-31
		"172.17.",
		"172.18.",
		"172.19.",
		"172.20.",
		"172.21.",
		"172.22.",
		"172.23.",
		"172.24.",
		"172.25.",
		"172.26.",
		"172.27.",
		"172.28.",
		"172.29.",
		"172.30.",
		"172.31.",
		"192.168.", // 192.168.0.0/16
	}
	for _, prefix := range privatePrefixes {
		if strings.HasPrefix(ip, prefix) {
			return true
		}
	}
	return false
}

func limaFileExists(path string) (bool, error) {
	if strings.TrimSpace(path) == "" {
		return false, ErrInvalidArgument
	}
	cmd := exec.Command(
		"limactl",
		"shell",
		"--tty=false",
		"default",
		"--",
		"test",
		"-f",
		path,
	)
	if err := cmd.Run(); err == nil {
		return true, nil
	} else if exitErr, ok := err.(*exec.ExitError); ok {
		if exitErr.ExitCode() == 1 {
			return false, nil
		}
		return false, fmt.Errorf("lima test failed for %s: %w", path, err)
	} else {
		return false, fmt.Errorf("lima test failed for %s: %w", path, err)
	}
}
