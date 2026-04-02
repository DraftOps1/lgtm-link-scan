package model

import "time"

type ScanResult struct {
	GeneratedAt       time.Time               `json:"generated_at"`
	Partial           bool                    `json:"partial"`
	ImplementedChecks []string                `json:"implemented_checks"`
	Service           string                  `json:"service,omitempty"`
	Lookback          string                  `json:"lookback"`
	Score             Score                   `json:"score"`
	Checks            []CategoryScore         `json:"checks"`
	Findings          []Finding               `json:"findings"`
	LogTrace          *LogTraceCheck          `json:"log_trace,omitempty"`
	MetricTrace       *MetricTraceCheck       `json:"metric_trace,omitempty"`
	TraceMetrics      *TraceMetricsCheck      `json:"trace_metrics,omitempty"`
	LabelsConsistency *LabelsConsistencyCheck `json:"labels_consistency,omitempty"`
}

type Score struct {
	Earned    int    `json:"earned"`
	Available int    `json:"available"`
	Percent   int    `json:"percent"`
	Severity  string `json:"severity"`
}

type CategoryScore struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Earned    int    `json:"earned"`
	Available int    `json:"available"`
	Percent   int    `json:"percent"`
	Status    string `json:"status"`
}

type LogTraceCheck struct {
	RequestedService      string   `json:"requested_service,omitempty"`
	RequestedServiceFound bool     `json:"requested_service_found"`
	ObservedServices      []string `json:"observed_services,omitempty"`
	SampledLogs           int      `json:"sampled_logs"`
	LogsWithTraceID       int      `json:"logs_with_trace_id"`
	LogsWithSpanID        int      `json:"logs_with_span_id"`
	TraceIDsChecked       int      `json:"trace_ids_checked"`
	TraceIDsResolved      int      `json:"trace_ids_resolved"`
	TraceCoveragePercent  float64  `json:"trace_coverage_percent"`
	SpanCoveragePercent   float64  `json:"span_coverage_percent"`
	ResolutionPercent     float64  `json:"resolution_percent"`
}

type MetricTraceCheck struct {
	RequestedService        string   `json:"requested_service,omitempty"`
	ServiceLabelKey         string   `json:"service_label_key"`
	Selector                string   `json:"selector"`
	SeriesMatched           int      `json:"series_matched"`
	ObservedMetrics         []string `json:"observed_metrics,omitempty"`
	SeriesWithExemplars     int      `json:"series_with_exemplars"`
	ExemplarMetrics         []string `json:"exemplar_metrics,omitempty"`
	ExemplarsFound          int      `json:"exemplars_found"`
	TraceIDsChecked         int      `json:"trace_ids_checked"`
	TraceIDsResolved        int      `json:"trace_ids_resolved"`
	ExemplarCoveragePercent float64  `json:"exemplar_coverage_percent"`
	ResolutionPercent       float64  `json:"resolution_percent"`
}

type TraceMetricsCheck struct {
	RequestedService       string   `json:"requested_service,omitempty"`
	ServiceLabelKey        string   `json:"service_label_key"`
	TraceQuery             string   `json:"trace_query"`
	MetricSelector         string   `json:"metric_selector"`
	TraceServices          []string `json:"trace_services,omitempty"`
	MetricServices         []string `json:"metric_services,omitempty"`
	ObservedMetrics        []string `json:"observed_metrics,omitempty"`
	RequiredMetricNames    []string `json:"required_metric_names,omitempty"`
	MatchedRequiredMetrics []string `json:"matched_required_metrics,omitempty"`
	MissingRequiredMetrics []string `json:"missing_required_metrics,omitempty"`
	ServiceNameAligned     bool     `json:"service_name_aligned"`
}

type LabelsConsistencyCheck struct {
	RequestedService             string                       `json:"requested_service,omitempty"`
	ServiceLabelKey              string                       `json:"service_label_key"`
	RequiredLabels               []string                     `json:"required_labels,omitempty"`
	LogsRequestedServiceFound    bool                         `json:"logs_requested_service_found"`
	MetricsRequestedServiceFound bool                         `json:"metrics_requested_service_found"`
	TracesRequestedServiceFound  bool                         `json:"traces_requested_service_found"`
	ObservedLogServices          []string                     `json:"observed_log_services,omitempty"`
	ObservedMetricServices       []string                     `json:"observed_metric_services,omitempty"`
	ObservedTraceServices        []string                     `json:"observed_trace_services,omitempty"`
	ObservedLabels               map[string]SignalLabelValues `json:"observed_labels,omitempty"`
	ConsistentLabels             []string                     `json:"consistent_labels,omitempty"`
	MissingLabels                []string                     `json:"missing_labels,omitempty"`
	MismatchedLabels             []string                     `json:"mismatched_labels,omitempty"`
	ConsistencyPercent           float64                      `json:"consistency_percent"`
}

type SignalLabelValues struct {
	Logs    []string `json:"logs,omitempty"`
	Metrics []string `json:"metrics,omitempty"`
	Traces  []string `json:"traces,omitempty"`
}
