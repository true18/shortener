package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const defaultHTTPTimeout = 5 * time.Second

// HTTPObserver posts audit events to a remote HTTP endpoint.
type HTTPObserver struct {
	endpoint string
	client   *http.Client
}

// NewHTTPObserver creates an observer that reuses the supplied HTTP client.
func NewHTTPObserver(endpoint string, client *http.Client) (*HTTPObserver, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("parse audit URL: %w", err)
	}
	if parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, fmt.Errorf("invalid audit URL %q", endpoint)
	}
	if client == nil {
		client = &http.Client{Timeout: defaultHTTPTimeout}
	}

	return &HTTPObserver{endpoint: endpoint, client: client}, nil
}

// Notify posts one JSON event and requires a successful HTTP status.
func (o *HTTPObserver) Notify(ctx context.Context, event Event) error {
	body, err := json.Marshal(event)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("audit server returned %s", resp.Status)
	}

	return nil
}
