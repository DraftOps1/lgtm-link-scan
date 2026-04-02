package clients

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type LokiClient struct {
	baseClient
}

type LokiLogEntry struct {
	Timestamp time.Time
	Line      string
}

type lokiQueryResponse struct {
	Data struct {
		Result []struct {
			Values [][]string `json:"values"`
		} `json:"result"`
	} `json:"data"`
}

func NewLokiClient(cfg ClientConfig) (*LokiClient, error) {
	base, err := newBaseClient(cfg)
	if err != nil {
		return nil, err
	}

	return &LokiClient{baseClient: base}, nil
}

func (c *LokiClient) Ready(ctx context.Context) (string, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/ready", nil)
	if err != nil {
		return "", err
	}

	ready, err := c.doText(req)
	if err != nil {
		return "", fmt.Errorf("loki ready check: %w", err)
	}

	return ready, nil
}

func (c *LokiClient) CountStreams(ctx context.Context, start, end time.Time) (int, error) {
	query := url.Values{
		"query":     []string{`{job=~".+"}`},
		"limit":     []string{"1"},
		"direction": []string{"backward"},
		"start":     []string{strconv.FormatInt(start.UnixNano(), 10)},
		"end":       []string{strconv.FormatInt(end.UnixNano(), 10)},
	}

	req, err := c.newRequest(ctx, http.MethodGet, "/loki/api/v1/query_range", query)
	if err != nil {
		return 0, err
	}

	var resp lokiQueryResponse
	if err := c.doJSON(req, &resp); err != nil {
		return 0, fmt.Errorf("loki query_range: %w", err)
	}

	return len(resp.Data.Result), nil
}

func (c *LokiClient) SampleLogs(ctx context.Context, start, end time.Time, limit int) ([]LokiLogEntry, error) {
	query := url.Values{
		"query":     []string{`{job=~".+"}`},
		"limit":     []string{strconv.Itoa(limit)},
		"direction": []string{"backward"},
		"start":     []string{strconv.FormatInt(start.UnixNano(), 10)},
		"end":       []string{strconv.FormatInt(end.UnixNano(), 10)},
	}

	req, err := c.newRequest(ctx, http.MethodGet, "/loki/api/v1/query_range", query)
	if err != nil {
		return nil, err
	}

	var resp lokiQueryResponse
	if err := c.doJSON(req, &resp); err != nil {
		return nil, fmt.Errorf("loki query_range: %w", err)
	}

	entries := make([]LokiLogEntry, 0, limit)
	for _, stream := range resp.Data.Result {
		for _, value := range stream.Values {
			if len(value) < 2 {
				continue
			}
			nanos, err := strconv.ParseInt(value[0], 10, 64)
			if err != nil {
				continue
			}
			entries = append(entries, LokiLogEntry{
				Timestamp: time.Unix(0, nanos).UTC(),
				Line:      value[1],
			})
			if len(entries) == limit {
				return entries, nil
			}
		}
	}

	return entries, nil
}
