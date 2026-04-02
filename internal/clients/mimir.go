package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type MimirClient struct {
	baseClient
}

type MetricSeries struct {
	Labels map[string]string
}

type Exemplar struct {
	Labels    map[string]string
	Timestamp time.Time
}

type ExemplarSeries struct {
	SeriesLabels map[string]string
	Exemplars    []Exemplar
}

type mimirQueryResponse struct {
	Data struct {
		Result []struct {
			Value []json.RawMessage `json:"value"`
		} `json:"result"`
	} `json:"data"`
}

type mimirSeriesResponse struct {
	Data []map[string]string `json:"data"`
}

type mimirExemplarResponse struct {
	Data []struct {
		SeriesLabels map[string]string `json:"seriesLabels"`
		Exemplars    []struct {
			Labels    map[string]string `json:"labels"`
			Timestamp float64           `json:"timestamp"`
		} `json:"exemplars"`
	} `json:"data"`
}

func NewMimirClient(cfg ClientConfig) (*MimirClient, error) {
	base, err := newBaseClient(cfg)
	if err != nil {
		return nil, err
	}

	return &MimirClient{baseClient: base}, nil
}

func (c *MimirClient) Ready(ctx context.Context) (string, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/-/ready", nil)
	if err != nil {
		return "", err
	}

	ready, err := c.doText(req)
	if err != nil {
		return "", fmt.Errorf("mimir ready check: %w", err)
	}

	return ready, nil
}

func (c *MimirClient) ActiveTargetsInLookback(ctx context.Context, lookback string) (float64, error) {
	query := url.Values{
		"query": []string{fmt.Sprintf("sum(max_over_time(up[%s]))", lookback)},
	}

	req, err := c.newRequest(ctx, http.MethodGet, "/api/v1/query", query)
	if err != nil {
		return 0, err
	}

	var resp mimirQueryResponse
	if err := c.doJSON(req, &resp); err != nil {
		return 0, fmt.Errorf("mimir query: %w", err)
	}

	if len(resp.Data.Result) == 0 || len(resp.Data.Result[0].Value) < 2 {
		return 0, nil
	}

	var sampleValue string
	if err := json.Unmarshal(resp.Data.Result[0].Value[1], &sampleValue); err != nil {
		return 0, fmt.Errorf("decode mimir sample value: %w", err)
	}

	value, err := strconv.ParseFloat(sampleValue, 64)
	if err != nil {
		return 0, fmt.Errorf("parse mimir sample value %q: %w", sampleValue, err)
	}

	return value, nil
}

func (c *MimirClient) MatchSeries(ctx context.Context, start, end time.Time, selector string) ([]MetricSeries, error) {
	query := url.Values{
		"match[]": []string{selector},
		"start":   []string{strconv.FormatFloat(float64(start.UnixNano())/1e9, 'f', -1, 64)},
		"end":     []string{strconv.FormatFloat(float64(end.UnixNano())/1e9, 'f', -1, 64)},
	}

	req, err := c.newRequest(ctx, http.MethodGet, "/api/v1/series", query)
	if err != nil {
		return nil, err
	}

	var resp mimirSeriesResponse
	if err := c.doJSON(req, &resp); err != nil {
		return nil, fmt.Errorf("mimir series query: %w", err)
	}

	series := make([]MetricSeries, 0, len(resp.Data))
	for _, item := range resp.Data {
		series = append(series, MetricSeries{Labels: item})
	}
	return series, nil
}

func (c *MimirClient) QueryExemplars(ctx context.Context, start, end time.Time, selector string) ([]ExemplarSeries, error) {
	query := url.Values{
		"query": []string{selector},
		"start": []string{strconv.FormatFloat(float64(start.UnixNano())/1e9, 'f', -1, 64)},
		"end":   []string{strconv.FormatFloat(float64(end.UnixNano())/1e9, 'f', -1, 64)},
	}

	req, err := c.newRequest(ctx, http.MethodGet, "/api/v1/query_exemplars", query)
	if err != nil {
		return nil, err
	}

	var resp mimirExemplarResponse
	if err := c.doJSON(req, &resp); err != nil {
		return nil, fmt.Errorf("mimir exemplar query: %w", err)
	}

	series := make([]ExemplarSeries, 0, len(resp.Data))
	for _, item := range resp.Data {
		exemplars := make([]Exemplar, 0, len(item.Exemplars))
		for _, exemplar := range item.Exemplars {
			exemplars = append(exemplars, Exemplar{
				Labels:    exemplar.Labels,
				Timestamp: time.Unix(0, int64(exemplar.Timestamp*float64(time.Second))).UTC(),
			})
		}
		series = append(series, ExemplarSeries{
			SeriesLabels: item.SeriesLabels,
			Exemplars:    exemplars,
		})
	}
	return series, nil
}
