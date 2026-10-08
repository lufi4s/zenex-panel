// Package helperclient sends fixed operations to the root helper over its Unix socket.
// The API never runs privileged commands itself.
package helperclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// Error is returned when the helper rejected or failed an operation.
// Its message is safe to show to the customer.
type Error struct{ Message string }

func (e *Error) Error() string { return e.Message }

type Client struct {
	http *http.Client
}

// New returns a client that connects to the helper socket at path.
func New(path string) *Client {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", path)
		},
	}
	return &Client{http: &http.Client{Transport: transport, Timeout: 16 * time.Minute}}
}

type request struct {
	Op   string            `json:"op"`
	Args map[string]string `json:"args"`
}

type response struct {
	OK     bool   `json:"ok"`
	UID    string `json:"uid,omitempty"`
	Output string `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
}

// Do runs one named operation and returns the account UID when the operation sets one.
func (c *Client) Do(ctx context.Context, op string, args map[string]string) (string, error) {
	return c.call(ctx, op, args, func(r response) string { return r.UID })
}

func (c *Client) call(ctx context.Context, op string, args map[string]string, pick func(response) string) (string, error) {
	body, err := json.Marshal(request{Op: op, Args: args})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://helper/v1/op", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("privileged helper is not reachable: %w", err)
	}
	defer resp.Body.Close()

	var out response
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&out); err != nil {
		return "", errors.New("privileged helper returned an unreadable response")
	}
	if !out.OK {
		msg := out.Error
		if msg == "" {
			msg = "operation failed"
		}
		return "", &Error{Message: msg}
	}
	return pick(out), nil
}
