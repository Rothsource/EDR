package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type RegisterRequest struct {
	Hostname        string `json:"hostname"`
	OS              string `json:"os"`
	EnrollmentToken string `json:"enrollment_token"`
	IPAddress       string `json:"ip_address,omitempty"`
	MACAddress      string `json:"mac_address,omitempty"`
}

type RegisterResponse struct {
	AgentID string `json:"agent_id"`
	APIKey  string `json:"api_key"`
}

type HeartbeatRequest struct {
	AgentID string `json:"agent_id"`
	APIKey  string `json:"api_key"`
}

type errorResponse struct {
	Detail string `json:"detail"`
}

var httpClient = &http.Client{Timeout: 10 * time.Second}

func Register(serverURL string, req RegisterRequest) (*RegisterResponse, error) {
	body, _ := json.Marshal(req)

	resp, err := httpClient.Post(serverURL+"/agent/register", "application/json", bytes.NewReader(body))
	if err != nil {
		// Never reached the server at all — DNS failure, wrong port, network down.
		return nil, fmt.Errorf("cannot reach server: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Reached the server fine — it logically rejected us (bad/expired/used token).
		var e errorResponse
		json.NewDecoder(resp.Body).Decode(&e)
		return nil, fmt.Errorf("server rejected registration: %s", e.Detail)
	}

	var result RegisterResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("could not parse server response: %w", err)
	}
	return &result, nil
}

func Heartbeat(serverURL string, req HeartbeatRequest) error {
	body, _ := json.Marshal(req)

	resp, err := httpClient.Post(serverURL+"/agent/heartbeat", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("cannot reach server: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var e errorResponse
		json.NewDecoder(resp.Body).Decode(&e)
		return fmt.Errorf("server rejected heartbeat: %s", e.Detail)
	}
	return nil
}
