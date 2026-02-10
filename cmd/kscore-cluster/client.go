// Package main implements the kscore-cluster CLI for cluster management operations.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"google.golang.org/grpc"
)

// ClusterClient provides access to cluster management operations.
type ClusterClient struct {
	grpcConn   *grpc.ClientConn
	httpClient *http.Client
	baseURL    string
}

// newClusterClient creates a new cluster client.
func newClusterClient(ctx context.Context) (*ClusterClient, error) {
	// Use HTTP REST API for cluster management.
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
	}

	baseURL := fmt.Sprintf("%s://%s/api/v1/cluster", getAPIScheme(serverAddr), serverAddr)

	return &ClusterClient{
		httpClient: httpClient,
		baseURL:    baseURL,
	}, nil
}

// Close closes the client connections.
func (c *ClusterClient) Close() error {
	if c.grpcConn != nil {
		return c.grpcConn.Close()
	}
	return nil
}

// GetStatus retrieves the cluster status.
func (c *ClusterClient) GetStatus(ctx context.Context) (*ClusterStatus, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/status", http.NoBody)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server error: %s - %s", resp.Status, string(body))
	}

	var status ClusterStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &status, nil
}

// GetMembers retrieves the list of cluster members.
func (c *ClusterClient) GetMembers(ctx context.Context) ([]MemberStatus, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/members", http.NoBody)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server error: %s - %s", resp.Status, string(body))
	}

	var members []MemberStatus
	if err := json.NewDecoder(resp.Body).Decode(&members); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	sortMembers(members)
	return members, nil
}

// GetLeader retrieves information about the current leader.
func (c *ClusterClient) GetLeader(ctx context.Context) (*MemberStatus, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/leader", http.NoBody)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // No leader
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server error: %s - %s", resp.Status, string(body))
	}

	var leader MemberStatus
	if err := json.NewDecoder(resp.Body).Decode(&leader); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &leader, nil
}

// AddMember adds a new member to the cluster.
func (c *ClusterClient) AddMember(ctx context.Context, address string) (string, error) {
	body := fmt.Sprintf(`{"address": %q}`, address)
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/members",
		strings.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("server error: %s - %s", resp.Status, string(respBody))
	}

	var result struct {
		MemberID string `json:"member_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	return result.MemberID, nil
}

// RemoveMember removes a member from the cluster.
func (c *ClusterClient) RemoveMember(ctx context.Context, memberID string, force bool) error {
	url := fmt.Sprintf("%s/members/%s", c.baseURL, memberID)
	if force {
		url += "?force=true"
	}

	req, err := http.NewRequestWithContext(ctx, "DELETE", url, http.NoBody)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server error: %s - %s", resp.Status, string(body))
	}

	return nil
}

// TransferLeader transfers leadership to the specified member.
func (c *ClusterClient) TransferLeader(ctx context.Context, targetID string) error {
	body := fmt.Sprintf(`{"target_id": %q}`, targetID)
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/leader/transfer",
		strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server error: %s - %s", resp.Status, string(respBody))
	}

	return nil
}

// Rebalance triggers a rebalance of agents across cluster members.
func (c *ClusterClient) Rebalance(ctx context.Context, reason string) (*RebalanceResult, error) {
	url := c.baseURL + "/rebalance"
	if reason != "" {
		url += "?reason=" + reason
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, http.NoBody)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server error: %s - %s", resp.Status, string(body))
	}

	var result RebalanceResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// Backup creates a backup of the cluster state.
func (c *ClusterClient) Backup(ctx context.Context) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/backup", http.NoBody)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server error: %s - %s", resp.Status, string(body))
	}

	return io.ReadAll(resp.Body)
}

// Restore restores cluster state from a backup.
func (c *ClusterClient) Restore(ctx context.Context, data []byte) error {
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/restore",
		bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server error: %s - %s", resp.Status, string(body))
	}

	return nil
}

// GetHealth retrieves detailed cluster health information.
func (c *ClusterClient) GetHealth(ctx context.Context) (*HealthReport, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/health", http.NoBody)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server error: %s - %s", resp.Status, string(body))
	}

	var health HealthReport
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &health, nil
}

// GetShards retrieves the shard assignments for agents.
func (c *ClusterClient) GetShards(ctx context.Context) (*ShardReport, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/shards", http.NoBody)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server error: %s - %s", resp.Status, string(body))
	}

	var shards ShardReport
	if err := json.NewDecoder(resp.Body).Decode(&shards); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &shards, nil
}

// JoinCluster joins this node to an existing cluster.
func (c *ClusterClient) JoinCluster(ctx context.Context, clusterAddr, token, advertiseAddr string) error {
	payload := map[string]string{"cluster_address": clusterAddr}
	if token != "" {
		payload["token"] = token
	}
	if advertiseAddr != "" {
		payload["advertise_addr"] = advertiseAddr
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/join",
		bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server error: %s - %s", resp.Status, string(respBody))
	}

	return nil
}

// RestartElection triggers a new leader election cycle.
func (c *ClusterClient) RestartElection(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/election/restart", http.NoBody)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server error: %s - %s", resp.Status, string(body))
	}

	return nil
}

// LeaveCluster removes this node from the cluster.
func (c *ClusterClient) LeaveCluster(ctx context.Context, force bool) error {
	url := c.baseURL + "/leave"
	if force {
		url += "?force=true"
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, http.NoBody)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server error: %s - %s", resp.Status, string(body))
	}

	return nil
}

// DrainMember drains agents from a cluster member.
func (c *ClusterClient) DrainMember(ctx context.Context, memberID string) error {
	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("%s/members/%s/drain", c.baseURL, memberID), http.NoBody)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server error: %s - %s", resp.Status, string(body))
	}

	return nil
}

// UndrainMember allows agents to be assigned to a cluster member again.
func (c *ClusterClient) UndrainMember(ctx context.Context, memberID string) error {
	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("%s/members/%s/undrain", c.baseURL, memberID), http.NoBody)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server error: %s - %s", resp.Status, string(body))
	}

	return nil
}

// getAPIScheme returns "https" by default, "http" only for localhost addresses.
// This ensures production deployments use TLS while allowing HTTP for local development.
func getAPIScheme(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		// If SplitHostPort fails, try treating the whole string as a host
		host = addr
	}
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return "http"
	}
	return "https"
}
