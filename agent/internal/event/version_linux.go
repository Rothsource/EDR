//go:build linux

package event

import (
	"bufio"
	"os"
	"strings"
)

// detectOSVersion returns a human-readable OS version, or "" if unknown
// (the server column is nullable). Reads /etc/os-release.
func detectOSVersion() string {
	f, err := os.Open("/etc/os-release")
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			return strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), `"`)
		}
	}
	return ""
}
