//go:build windows

package event

import (
	"fmt"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// detectOSVersion returns a human-readable Windows version string, e.g.
// "Windows 11 Pro 23H2 (build 22631.3155)", or "" if it can't be determined
// (the server column is nullable).
//
// ProductName alone is not trustworthy: Windows 11 machines still report
// ProductName = "Windows 10 ..." because Microsoft never updated that
// registry value after the Windows 11 release. Build 22000 was the first
// Windows 11 build, so the build number is what actually tells 10 and 11
// apart; ProductName is only used here as a source of the edition word
// ("Home"/"Pro"/...), not the major version.
func detectOSVersion() string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()

	productName, _, _ := k.GetStringValue("ProductName")

	// DisplayVersion (e.g. "23H2") is the current name for the feature
	// update string; ReleaseId is the older name for the same idea on
	// builds that predate DisplayVersion.
	displayVersion, _, err := k.GetStringValue("DisplayVersion")
	if err != nil {
		displayVersion, _, _ = k.GetStringValue("ReleaseId")
	}

	// CurrentBuildNumber is stored as a REG_SZ, not a DWORD.
	buildStr, _, _ := k.GetStringValue("CurrentBuildNumber")
	build := parseLeadingInt(buildStr)

	// UBR (Update Build Revision) supplies the ".3155" half of "22631.3155".
	// It's absent on some older images, so treat a lookup failure or 0 as
	// "leave it off" rather than printing a misleading ".0".
	ubr, _, ubrErr := k.GetIntegerValue("UBR")

	name := productName
	if build >= 22000 {
		if strings.Contains(name, "Windows 10") {
			name = strings.Replace(name, "Windows 10", "Windows 11", 1)
		} else {
			name = "Windows 11"
		}
	}
	if name == "" {
		name = "Windows"
	}

	out := name
	if displayVersion != "" {
		out += " " + displayVersion
	}
	if buildStr != "" {
		if ubrErr == nil && ubr != 0 {
			out += fmt.Sprintf(" (build %s.%d)", buildStr, ubr)
		} else {
			out += fmt.Sprintf(" (build %s)", buildStr)
		}
	}
	return out
}

// parseLeadingInt parses the leading run of ASCII digits in s, returning 0
// if s doesn't start with a digit. Used instead of strconv.Atoi because a
// malformed or missing registry value should degrade to "unknown build"
// (0, which fails the >= 22000 check) rather than panicking or erroring.
func parseLeadingInt(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}
