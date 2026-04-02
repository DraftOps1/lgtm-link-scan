package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const DefaultTimeout = 5 * time.Second

type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

type ClientConfig struct {
	BaseURL     string
	BearerToken string
	HTTPClient  HTTPDoer
}

type baseClient struct {
	baseURL     *url.URL
	bearerToken string
	httpClient  HTTPDoer
}

func newBaseClient(cfg ClientConfig) (baseClient, error) {
	rawURL := strings.TrimSpace(cfg.BaseURL)
	if rawURL == "" {
		return baseClient{}, fmt.Errorf("base URL is required")
	}

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return baseClient{}, fmt.Errorf("parse base URL %q: %w", rawURL, err)
	}
	if parsedURL.Scheme == "" || parsedURL.Host == "" {
		return baseClient{}, fmt.Errorf("base URL %q must include scheme and host", rawURL)
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: DefaultTimeout}
	}

	return baseClient{
		baseURL:     parsedURL,
		bearerToken: cfg.BearerToken,
		httpClient:  httpClient,
	}, nil
}

func (c baseClient) newRequest(ctx context.Context, method, endpointPath string, query url.Values) (*http.Request, error) {
	requestURL := *c.baseURL
	requestURL.Path = strings.TrimRight(c.baseURL.Path, "/") + "/" + strings.TrimLeft(endpointPath, "/")
	requestURL.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, method, requestURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build request %s %s: %w", method, requestURL.String(), err)
	}

	req.Header.Set("Accept", "application/json")
	if c.bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearerToken)
	}

	return req, nil
}

func (c baseClient) doJSON(req *http.Request, dst any) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := requireSuccess(req, resp); err != nil {
		return err
	}

	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		return fmt.Errorf("decode %s %s: %w", req.Method, req.URL.String(), err)
	}

	return nil
}

func (c baseClient) doText(req *http.Request) (string, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if err := requireSuccess(req, resp); err != nil {
		return "", err
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
	if err != nil {
		return "", fmt.Errorf("read %s %s: %w", req.Method, req.URL.String(), err)
	}

	return strings.TrimSpace(string(body)), nil
}

func requireSuccess(req *http.Request, resp *http.Response) error {
	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
		return nil
	}

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
	message := strings.TrimSpace(string(body))
	if message == "" {
		message = http.StatusText(resp.StatusCode)
	}

	return fmt.Errorf("%s %s: unexpected status %d: %s", req.Method, req.URL.String(), resp.StatusCode, message)
}
