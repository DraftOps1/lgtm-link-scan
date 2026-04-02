package clients

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestLokiClientReadyAndCountStreams(t *testing.T) {
	t.Parallel()

	httpClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			switch r.URL.Path {
			case "/proxy/loki/ready":
				if got := r.Header.Get("Authorization"); got != "Bearer secret-token" {
					t.Fatalf("authorization header = %q", got)
				}
				return jsonResponse(http.StatusOK, "ready"), nil
			case "/proxy/loki/loki/api/v1/query_range":
				if got := r.URL.Query().Get("query"); got != `{job=~".+"}` {
					t.Fatalf("query = %q", got)
				}
				if got := r.URL.Query().Get("limit"); got != "1" {
					t.Fatalf("limit = %q", got)
				}
				if got := r.Header.Get("Authorization"); got != "Bearer secret-token" {
					t.Fatalf("authorization header = %q", got)
				}
				return jsonResponse(http.StatusOK, `{"data":{"result":[{},{}]}}`), nil
			default:
				return jsonResponse(http.StatusNotFound, `{"error":"not found"}`), nil
			}
		}),
	}

	client, err := NewLokiClient(ClientConfig{
		BaseURL:     "http://example.invalid/proxy/loki",
		BearerToken: "secret-token",
		HTTPClient:  httpClient,
	})
	if err != nil {
		t.Fatalf("NewLokiClient: %v", err)
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

	streams, err := client.CountStreams(ctx, time.Unix(100, 0), time.Unix(200, 0))
	if err != nil {
		t.Fatalf("CountStreams: %v", err)
	}
	if streams != 2 {
		t.Fatalf("streams = %d", streams)
	}
}

func TestLokiClientSampleLogs(t *testing.T) {
	t.Parallel()

	httpClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			switch r.URL.Path {
			case "/proxy/loki/loki/api/v1/query_range":
				if got := r.URL.Query().Get("limit"); got != "2" {
					t.Fatalf("limit = %q", got)
				}
				return jsonResponse(http.StatusOK, `{"data":{"result":[{"values":[["1000000000","{\"msg\":\"one\"}"],["2000000000","{\"msg\":\"two\"}"]]}]}}`), nil
			default:
				return jsonResponse(http.StatusNotFound, `{"error":"not found"}`), nil
			}
		}),
	}

	client, err := NewLokiClient(ClientConfig{
		BaseURL:    "http://example.invalid/proxy/loki",
		HTTPClient: httpClient,
	})
	if err != nil {
		t.Fatalf("NewLokiClient: %v", err)
	}

	entries, err := client.SampleLogs(context.Background(), time.Unix(100, 0), time.Unix(200, 0), 2)
	if err != nil {
		t.Fatalf("SampleLogs: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %d", len(entries))
	}
	if entries[0].Line != `{"msg":"one"}` {
		t.Fatalf("unexpected first line: %q", entries[0].Line)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
