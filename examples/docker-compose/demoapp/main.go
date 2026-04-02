package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

type appConfig struct {
	Addr                  string
	TraceServiceName      string
	LogServiceName        string
	MetricServiceName     string
	ServiceNamespace      string
	DeploymentEnvironment string
	LogCorrelationMode    string
	ExemplarMode          string
	LogFile               string
}

type application struct {
	cfg             appConfig
	logger          *slog.Logger
	tracer          trace.Tracer
	requestDuration *prometheus.HistogramVec
	requestsTotal   *prometheus.CounterVec
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(status int) {
	s.status = status
	s.ResponseWriter.WriteHeader(status)
}

func main() {
	rand.Seed(time.Now().UnixNano())

	cfg := loadConfig()
	logger, closeLog, err := newLogger(cfg.LogFile)
	if err != nil {
		exitWithError(fmt.Errorf("create logger: %w", err))
	}
	defer closeLog()

	ctx := context.Background()
	shutdownTracing, err := setupTracing(ctx, cfg)
	if err != nil {
		exitWithError(fmt.Errorf("setup tracing: %w", err))
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = shutdownTracing(shutdownCtx)
	}()

	app := newApplication(cfg, logger)

	server := &http.Server{
		Addr:              cfg.Addr,
		Handler:           app.routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Info("demo app listening", "addr", cfg.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server exited", "error", err.Error())
		}
	}()

	stop := make(chan os.Signal, 1)
	signalNotify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownCtx)
}

func signalNotify(ch chan<- os.Signal, sig ...os.Signal) {
	signal.Notify(ch, sig...)
}

func loadConfig() appConfig {
	return appConfig{
		Addr:                  envOrDefault("APP_ADDR", ":8080"),
		TraceServiceName:      envOrDefault("TRACE_SERVICE_NAME", "checkout"),
		LogServiceName:        envOrDefault("LOG_SERVICE_NAME", "checkout"),
		MetricServiceName:     envOrDefault("METRIC_SERVICE_NAME", "checkout"),
		ServiceNamespace:      envOrDefault("SERVICE_NAMESPACE", "store"),
		DeploymentEnvironment: envOrDefault("DEPLOYMENT_ENVIRONMENT", "demo"),
		LogCorrelationMode:    envOrDefault("LOG_CORRELATION_MODE", "missing"),
		ExemplarMode:          envOrDefault("EXEMPLAR_MODE", "disabled"),
		LogFile:               envOrDefault("LOG_FILE", "/tmp/demoapp/app.log"),
	}
}

func newLogger(logFile string) (*slog.Logger, func(), error) {
	if err := os.MkdirAll(filepath.Dir(logFile), 0o755); err != nil {
		return nil, nil, fmt.Errorf("create log dir: %w", err)
	}

	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, nil, fmt.Errorf("open log file: %w", err)
	}

	writer := io.MultiWriter(os.Stdout, f)
	logger := slog.New(slog.NewJSONHandler(writer, &slog.HandlerOptions{Level: slog.LevelInfo}))

	return logger, func() {
		_ = f.Close()
	}, nil
}

func setupTracing(ctx context.Context, cfg appConfig) (func(context.Context) error, error) {
	exporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("create otlp trace exporter: %w", err)
	}

	resource, err := sdkresource.New(
		ctx,
		sdkresource.WithAttributes(
			attribute.String("service.name", cfg.TraceServiceName),
			attribute.String("service.namespace", cfg.ServiceNamespace),
			attribute.String("deployment.environment", cfg.DeploymentEnvironment),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create otel resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	return tp.Shutdown, nil
}

func newApplication(cfg appConfig, logger *slog.Logger) *application {
	requestDuration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "demo_request_duration_seconds",
			Help:    "HTTP request latency for the demo application.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"service_name", "service_namespace", "deployment_environment", "route", "status_code"},
	)

	requestsTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "demo_requests_total",
			Help: "HTTP request count for the demo application.",
		},
		[]string{"service_name", "service_namespace", "deployment_environment", "route", "status_code"},
	)

	return &application{
		cfg:             cfg,
		logger:          logger,
		tracer:          otel.Tracer("demoapp"),
		requestDuration: requestDuration,
		requestsTotal:   requestsTotal,
	}
}

func (a *application) routes() http.Handler {
	mux := http.NewServeMux()
	registry := prometheus.NewRegistry()
	registry.MustRegister(a.requestDuration, a.requestsTotal)
	mux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	}))
	mux.Handle("/healthz", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	mux.Handle("/checkout", a.instrument("checkout", a.handleCheckout))
	return mux
}

func (a *application) instrument(route string, next func(context.Context, http.ResponseWriter, *http.Request) error) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		ctx, span := a.tracer.Start(
			ctx,
			route,
			trace.WithAttributes(
				attribute.String("http.method", r.Method),
				attribute.String("http.route", route),
			),
		)
		defer span.End()

		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		if err := next(ctx, recorder, r); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			if recorder.status < http.StatusBadRequest {
				recorder.WriteHeader(http.StatusInternalServerError)
			}
			a.logWithTrace(ctx, slog.LevelError, "request failed", "route", route, "error", err.Error())
		}

		a.observe(ctx, route, recorder.status, time.Since(start))
	})
}

func (a *application) handleCheckout(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	sku := r.URL.Query().Get("sku")
	if sku == "" {
		sku = "sku-123"
	}

	a.logWithTrace(ctx, slog.LevelInfo, "checkout started", "route", "checkout", "sku", sku)

	ctx, paymentSpan := a.tracer.Start(ctx, "payment.authorize")
	paymentSpan.SetAttributes(attribute.String("sku", sku))
	time.Sleep(time.Duration(50+rand.Intn(200)) * time.Millisecond)

	if rand.Intn(10) == 0 {
		err := errors.New("payment provider timeout")
		paymentSpan.RecordError(err)
		paymentSpan.SetStatus(codes.Error, err.Error())
		paymentSpan.End()
		return err
	}

	paymentSpan.SetStatus(codes.Ok, "")
	paymentSpan.End()

	resp := map[string]any{
		"ok":   true,
		"sku":  sku,
		"mode": a.cfg.LogCorrelationMode + "/" + a.cfg.ExemplarMode,
	}

	a.logWithTrace(ctx, slog.LevelInfo, "checkout completed", "route", "checkout", "sku", sku)

	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(resp)
}

func (a *application) observe(ctx context.Context, route string, status int, duration time.Duration) {
	labels := []string{
		a.cfg.MetricServiceName,
		a.cfg.ServiceNamespace,
		a.cfg.DeploymentEnvironment,
		route,
		strconv.Itoa(status),
	}

	a.requestsTotal.WithLabelValues(labels...).Inc()

	observer := a.requestDuration.WithLabelValues(labels...)
	if a.cfg.ExemplarMode == "enabled" {
		if exemplarObserver, ok := observer.(prometheus.ExemplarObserver); ok {
			if exemplar := exemplarLabels(ctx); exemplar != nil {
				exemplarObserver.ObserveWithExemplar(duration.Seconds(), exemplar)
				return
			}
		}
	}

	observer.Observe(duration.Seconds())
}

func exemplarLabels(ctx context.Context) prometheus.Labels {
	spanCtx := trace.SpanContextFromContext(ctx)
	if !spanCtx.IsValid() {
		return nil
	}

	return prometheus.Labels{
		"trace_id": spanCtx.TraceID().String(),
	}
}

func (a *application) logWithTrace(ctx context.Context, level slog.Level, message string, args ...any) {
	attrs := []any{
		"service_name", a.cfg.LogServiceName,
		"service_namespace", a.cfg.ServiceNamespace,
		"deployment_environment", a.cfg.DeploymentEnvironment,
	}

	if a.cfg.LogCorrelationMode == "full" {
		spanCtx := trace.SpanContextFromContext(ctx)
		if spanCtx.IsValid() {
			attrs = append(attrs,
				"trace_id", spanCtx.TraceID().String(),
				"span_id", spanCtx.SpanID().String(),
			)
		}
	}

	attrs = append(attrs, args...)
	a.logger.Log(ctx, level, message, attrs...)
}

func envOrDefault(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func exitWithError(err error) {
	_, _ = fmt.Fprintf(os.Stderr, "%v\n", err)
	os.Exit(1)
}
