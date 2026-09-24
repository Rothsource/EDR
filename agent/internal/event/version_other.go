//go:build !linux && !windows

package event

// detectOSVersion has no implementation for this platform. The server
// column is nullable, so "" is a valid, honest answer here.
func detectOSVersion() string {
	return ""
}
