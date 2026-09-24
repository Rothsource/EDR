//go:build !linux && !windows

package core

import (
	"context"

	"khemstrix-agent/internal/wsclient"
)

var AuthConfigPath = ""

type AuthCollectorHandle struct{}

func (h *AuthCollectorHandle) Reload() {}

func StartAuthCollector(ctx context.Context, agentID string, wsc *wsclient.Client) *AuthCollectorHandle {
	return &AuthCollectorHandle{}
}
