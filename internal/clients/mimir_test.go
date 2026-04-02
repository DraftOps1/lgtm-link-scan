package clients

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestMimirClientReadyAndActiveTargets(t *testing.T) {
	t.Parallel()

	httpClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			switch r.URL.Path {
			case "/prometheus/-/ready":
				return jsonResponse(http.StatusOK, "ready"), nil
			case "/prometheus/api/v1/query":
				if got := r.URL.Query().Get("query"); got != "sum(max_over_time(up[30m]))" {
					t.Fatalf("query = %q", got)
				}
				if got := r.Header.Get("Authorization"); got != "Bearer secret-token" {
					t.Fatalf("authorization header = %q", got)
				}
				return jsonResponse(http.StatusOK, `{"data":{"result":[{"value":[123,"2"]}]}}`), nil
			default:
				return jsonResponse(http.StatusNotFound, `{"error":"not found"}`), nil
			}
		}),
	}

	client, err := NewMimirClient(ClientConfig{
		BaseURL:     "http://example.invalid/prometheus",
		BearerToken: "secret-token",
		HTTPClient:  httpClient,
	})
	if err != nil {
		t.Fatalf("NewMimirClient: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	ready, err := client.Ready(ctx)
	if err != nil {
		t.Fatalf("Ready: %v", err)
	}
	if ready != "ready" {
		t.Fatalf("ready = %q", ready)
	}

	targets, err := client.ActiveTargetsInLookback(ctx, "30m")
	if err != nil {
		t.Fatalf("ActiveTargetsInLookback: %v", err)
	}
	if targets != 2 {
		t.Fatalf("targets = %v", targets)
	}
}

func TestMimirClientSeriesAndExemplars(t *testing.T) {
	t.Parallel()

	httpClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			switch r.URL.Path {
			case "/prometheus/api/v1/series":
				if got := r.URL.Query().Get("match[]"); got != `{service_name="checkout"}` {
					t.Fatalf("match[] = %q", got)
				}
				return jsonResponse(http.StatusOK, `{"data":[{"__name__":"demo_request_duration_seconds_bucket","service_name":"checkout"}]}`), nil
			case "/prometheus/api/v1/query_exemplars":
				if got := r.URL.Query().Get("query"); got != `{service_name="checkout"}` {
					t.Fatalf("query = %q", got)
				}
				return jsonResponse(http.StatusOK, `{"data":[{"seriesLabels":{"__name__":"demo_request_duration_seconds_bucket"},"exemplars":[{"labels":{"trace_id":"trace-1"},"timestamp":123.4}]}]}`), nil
			default:
				return jsonResponse(http.StatusNotFound, `{"error":"not found"}`), nil
			}
		}),
	}

	client, err := NewMimirClient(ClientConfig{
		BaseURL:    "http://example.invalid/prometheus",
		HTTPClient: httpClient,
	})
	if err != nil {
		t.Fatalf("NewMimirClient: %v", err)
	}

	series, err := client.MatchSeries(context.Background(), time.Unix(100, 0), time.Unix(200, 0), `{service_name="checkout"}`)
	if err != nil {
		t.Fatalf("MatchSeries: %v", err)
	}
	if len(series) != 1 || series[0].Labels["__name__"] != "demo_request_duration_seconds_bucket" {
		t.Fatalf("unexpected series: %+v", series)
	}

	exemplars, err := client.QueryExemplars(context.Background(), time.Unix(100, 0), time.Unix(200, 0), `{service_name="checkout"}`)
	if err != nil {
		t.Fatalf("QueryExemplars: %v", err)
	}
	if len(exemplars) != 1 || len(exemplars[0].Exemplars) != 1 {
		t.Fatalf("unexpected exemplars: %+v", exemplars)
	}
}
