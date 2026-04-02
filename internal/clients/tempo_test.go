package clients

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestTempoClientReadyAndSearchTraceQL(t *testing.T) {
	t.Parallel()

	httpClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			switch r.URL.Path {
			case "/tempo/ready":
				return jsonResponse(http.StatusOK, "ready"), nil
			case "/tempo/api/search":
				if got := r.URL.Query().Get("q"); got != `{ resource.service.name = "checkout" }` {
					t.Fatalf("query = %q", got)
				}
				if got := r.URL.Query().Get("limit"); got != "2" {
					t.Fatalf("limit = %q", got)
				}
				if got := r.Header.Get("Authorization"); got != "Bearer secret-token" {
					t.Fatalf("authorization header = %q", got)
				}
				return jsonResponse(http.StatusOK, `{"traces":[{},{}]}`), nil
			default:
				return jsonResponse(http.StatusNotFound, `{"error":"not found"}`), nil
			}
		}),
	}

	client, err := NewTempoClient(ClientConfig{
		BaseURL:     "http://example.invalid/tempo",
		BearerToken: "secret-token",
		HTTPClient:  httpClient,
	})
	if err != nil {
		t.Fatalf("NewTempoClient: %v", err)
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

	traces, err := client.SearchTraceQL(ctx, time.Unix(100, 0), time.Unix(200, 0), `{ resource.service.name = "checkout" }`, 2)
	if err != nil {
		t.Fatalf("SearchTraceQL: %v", err)
	}
	if traces != 2 {
		t.Fatalf("traces = %d", traces)
	}
}

func TestTempoClientTraceExists(t *testing.T) {
	t.Parallel()

	httpClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			switch r.URL.Path {
			case "/tempo/api/v2/traces/trace-present":
				return jsonResponse(http.StatusOK, `{"trace":{"resourceSpans":[{}]}}`), nil
			case "/tempo/api/v2/traces/trace-missing":
				return jsonResponse(http.StatusOK, `{"trace":{}}`), nil
			default:
				return jsonResponse(http.StatusNotFound, `{"error":"not found"}`), nil
			}
		}),
	}

	client, err := NewTempoClient(ClientConfig{
		BaseURL:    "http://example.invalid/tempo",
		HTTPClient: httpClient,
	})
	if err != nil {
		t.Fatalf("NewTempoClient: %v", err)
	}

	present, err := client.TraceExists(context.Background(), "trace-present")
	if err != nil {
		t.Fatalf("TraceExists(trace-present): %v", err)
	}
	if !present {
		t.Fatal("trace-present should exist")
	}

	missing, err := client.TraceExists(context.Background(), "trace-missing")
	if err != nil {
		t.Fatalf("TraceExists(trace-missing): %v", err)
	}
	if missing {
		t.Fatal("trace-missing should not exist")
	}
}

func TestTempoClientSearchServiceNames(t *testing.T) {
	t.Parallel()

	httpClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			switch r.URL.Path {
			case "/tempo/api/search":
				return jsonResponse(http.StatusOK, `{"traces":[{"rootServiceName":"checkout","serviceStats":{"checkout":{"spanCount":2},"payment":{"spanCount":1}}}]}`), nil
			default:
				return jsonResponse(http.StatusNotFound, `{"error":"not found"}`), nil
			}
		}),
	}

	client, err := NewTempoClient(ClientConfig{
		BaseURL:    "http://example.invalid/tempo",
		HTTPClient: httpClient,
	})
	if err != nil {
		t.Fatalf("NewTempoClient: %v", err)
	}

	services, err := client.SearchServiceNames(context.Background(), time.Unix(100, 0), time.Unix(200, 0), `{ resource.service.name = "checkout" }`, 5)
	if err != nil {
		t.Fatalf("SearchServiceNames: %v", err)
	}
	if len(services) != 2 || services[0] != "checkout" || services[1] != "payment" {
		t.Fatalf("unexpected services: %+v", services)
	}
}

func TestTempoClientSampleTraceResourceAttributes(t *testing.T) {
	t.Parallel()

	httpClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			switch r.URL.Path {
			case "/tempo/api/search":
				return jsonResponse(http.StatusOK, `{"traces":[{"traceID":"trace-1"}]}`), nil
			case "/tempo/api/v2/traces/trace-1":
				return jsonResponse(http.StatusOK, `{"trace":{"resourceSpans":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"checkout"}},{"key":"service.namespace","value":{"stringValue":"store"}}]}}]}}`), nil
			default:
				return jsonResponse(http.StatusNotFound, `{"error":"not found"}`), nil
			}
		}),
	}

	client, err := NewTempoClient(ClientConfig{
		BaseURL:    "http://example.invalid/tempo",
		HTTPClient: httpClient,
	})
	if err != nil {
		t.Fatalf("NewTempoClient: %v", err)
	}

	attributes, err := client.SampleTraceResourceAttributes(context.Background(), time.Unix(100, 0), time.Unix(200, 0), `{ resource.service.name = "checkout" }`, 5)
	if err != nil {
		t.Fatalf("SampleTraceResourceAttributes: %v", err)
	}
	if len(attributes) != 1 || attributes[0]["service.name"] != "checkout" || attributes[0]["service.namespace"] != "store" {
		t.Fatalf("unexpected attributes: %+v", attributes)
	}
}
