//go:build !linux

package core

import (
	"context"

	"khemstrix-agent/internal/wsclient"
)

// No-op until parser_windows.go (Step 6) exists.
func StartAuthCollector(ctx context.Context, agentID string, wsc *wsclient.Client) {
}
