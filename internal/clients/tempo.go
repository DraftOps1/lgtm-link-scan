package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

type TempoClient struct {
	baseClient
}

type tempoSearchResponse struct {
	Traces []struct {
		TraceID         string                     `json:"traceID"`
		RootServiceName string                     `json:"rootServiceName"`
		ServiceStats    map[string]json.RawMessage `json:"serviceStats"`
	} `json:"traces"`
}

type tempoTraceResponse struct {
	Trace struct {
		ResourceSpans []struct {
			Resource struct {
				Attributes []tempoAttribute `json:"attributes"`
			} `json:"resource"`
		} `json:"resourceSpans"`
	} `json:"trace"`
}

type tempoAttribute struct {
	Key   string `json:"key"`
	Value struct {
		StringValue string `json:"stringValue"`
	} `json:"value"`
}

func NewTempoClient(cfg ClientConfig) (*TempoClient, error) {
	base, err := newBaseClient(cfg)
	if err != nil {
		return nil, err
	}

	return &TempoClient{baseClient: base}, nil
}

func (c *TempoClient) Ready(ctx context.Context) (string, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/ready", nil)
	if err != nil {
		return "", err
	}

	ready, err := c.doText(req)
	if err != nil {
		return "", fmt.Errorf("tempo ready check: %w", err)
	}

	return ready, nil
}

func (c *TempoClient) SearchTraceQL(ctx context.Context, start, end time.Time, query string, limit int) (int, error) {
	params := url.Values{
		"start": []string{strconv.FormatInt(start.Unix(), 10)},
		"end":   []string{strconv.FormatInt(end.Unix(), 10)},
		"limit": []string{strconv.Itoa(limit)},
	}
	if query != "" {
		params.Set("q", query)
	}

	req, err := c.newRequest(ctx, http.MethodGet, "/api/search", params)
	if err != nil {
		return 0, err
	}

	var resp tempoSearchResponse
	if err := c.doJSON(req, &resp); err != nil {
		return 0, fmt.Errorf("tempo search: %w", err)
	}

	return len(resp.Traces), nil
}

func (c *TempoClient) SearchServiceNames(ctx context.Context, start, end time.Time, query string, limit int) ([]string, error) {
	params := url.Values{
		"start": []string{strconv.FormatInt(start.Unix(), 10)},
		"end":   []string{strconv.FormatInt(end.Unix(), 10)},
		"limit": []string{strconv.Itoa(limit)},
	}
	if query != "" {
		params.Set("q", query)
	}

	req, err := c.newRequest(ctx, http.MethodGet, "/api/search", params)
	if err != nil {
		return nil, err
	}

	var resp tempoSearchResponse
	if err := c.doJSON(req, &resp); err != nil {
		return nil, fmt.Errorf("tempo search services: %w", err)
	}

	services := map[string]struct{}{}
	for _, trace := range resp.Traces {
		if trace.RootServiceName != "" {
			services[trace.RootServiceName] = struct{}{}
		}
		for serviceName := range trace.ServiceStats {
			if serviceName != "" {
				services[serviceName] = struct{}{}
			}
		}
	}

	result := make([]string, 0, len(services))
	for serviceName := range services {
		result = append(result, serviceName)
	}
	sort.Strings(result)
	return result, nil
}

func (c *TempoClient) TraceExists(ctx context.Context, traceID string) (bool, error) {
	lookup, err := c.lookupTrace(ctx, traceID)
	if err != nil {
		return false, err
	}

	return len(lookup.Trace.ResourceSpans) > 0, nil
}

func (c *TempoClient) SampleTraceResourceAttributes(ctx context.Context, start, end time.Time, query string, limit int) ([]map[string]string, error) {
	params := url.Values{
		"start": []string{strconv.FormatInt(start.Unix(), 10)},
		"end":   []string{strconv.FormatInt(end.Unix(), 10)},
		"limit": []string{strconv.Itoa(limit)},
	}
	if query != "" {
		params.Set("q", query)
	}

	req, err := c.newRequest(ctx, http.MethodGet, "/api/search", params)
	if err != nil {
		return nil, err
	}

	var resp tempoSearchResponse
	if err := c.doJSON(req, &resp); err != nil {
		return nil, fmt.Errorf("tempo search trace attributes: %w", err)
	}

	result := make([]map[string]string, 0, len(resp.Traces))
	for _, trace := range resp.Traces {
		if trace.TraceID == "" {
			continue
		}

		attributes, err := c.TraceResourceAttributes(ctx, trace.TraceID)
		if err != nil {
			return nil, err
		}
		if len(attributes) > 0 {
			result = append(result, attributes)
		}
	}

	return result, nil
}

func (c *TempoClient) TraceResourceAttributes(ctx context.Context, traceID string) (map[string]string, error) {
	lookup, err := c.lookupTrace(ctx, traceID)
	if err != nil {
		return nil, err
	}
	if len(lookup.Trace.ResourceSpans) == 0 {
		return nil, nil
	}

	attributes := map[string]string{}
	for _, resourceSpan := range lookup.Trace.ResourceSpans {
		for _, attribute := range resourceSpan.Resource.Attributes {
			if strings.TrimSpace(attribute.Key) == "" || strings.TrimSpace(attribute.Value.StringValue) == "" {
				continue
			}
			attributes[attribute.Key] = attribute.Value.StringValue
		}
	}

	return attributes, nil
}

func (c *TempoClient) lookupTrace(ctx context.Context, traceID string) (tempoTraceResponse, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/api/v2/traces/"+traceID, nil)
	if err != nil {
		return tempoTraceResponse{}, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return tempoTraceResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return tempoTraceResponse{}, nil
	}
	if err := requireSuccess(req, resp); err != nil {
		return tempoTraceResponse{}, fmt.Errorf("tempo trace lookup: %w", err)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return tempoTraceResponse{}, fmt.Errorf("read tempo trace lookup response: %w", err)
	}

	var lookup tempoTraceResponse
	if err := json.Unmarshal(body, &lookup); err != nil {
		return tempoTraceResponse{}, fmt.Errorf("decode tempo trace lookup response: %w", err)
	}

	return lookup, nil
}
