package auth

// Placeholder OCSF values; the server decoder overwrites these (Section 7A/D-decisions).
const (
	placeholderClassUID    = 3002
	placeholderCategoryUID = 3
	placeholderActivityID  = 0
	placeholderSeverityID  = 1
)

// authConfig mirrors config/auth.json (Section 7B).
type authConfig struct {
	Sources []string `json:"sources"`
	Version int      `json:"version"`
	Source  string   `json:"source"` // "server" | "local"
}
