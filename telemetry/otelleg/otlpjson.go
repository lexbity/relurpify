package otelleg

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// Signal identifiers for the OTLP/HTTP wire paths. They also key the file
// transport's decoder.
const (
	signalTraces  = "traces"
	signalLogs    = "logs"
	signalMetrics = "metrics"

	// localEndpointHost is a synthetic host for the file exporters: their
	// requests never leave the process because the transport is the segment
	// writer, but the OTLP/HTTP exporters require a syntactically valid
	// endpoint. The signal path (/v1/traces, /v1/logs, /v1/metrics) is appended
	// by the exporter, matching the collector leg.
	localEndpointHost = "127.0.0.1:4318"
)

// endpointConfig carries the network-leg connection settings shared by the three
// OTLP/HTTP exporters.
type endpointConfig struct {
	endpoint string
	headers  map[string]string
	timeout  time.Duration
	insecure bool
	caFile   string
}

// fileTransport is an http.RoundTripper that writes the OTLP/HTTP request body
// to the local segment store instead of dialing a collector. The body is the
// protobuf Export*ServiceRequest the official OTLP exporter produced, so
// decoding it and re-encoding with protojson yields canonical OTLP-JSON with
// resourceSpans/resourceLogs/resourceMetrics at the top level (spec §6.5).
type fileTransport struct {
	signal string
	writer *segmentWriter
}

// RoundTrip implements http.RoundTripper. It never performs network I/O and
// always reports success so the batch exporters do not retry a local write.
func (t *fileTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, fmt.Errorf("read OTLP batch: %w", err)
	}
	if err := req.Body.Close(); err != nil {
		return nil, fmt.Errorf("close OTLP batch: %w", err)
	}
	line, err := encodeOTLPJSON(t.signal, body)
	if err != nil {
		return nil, err
	}
	line = append(line, '\n')
	// segmentWriter.Write never returns an error: disk failures are counted and
	// the leg disabled inside the writer.
	_, _ = t.writer.Write(line)
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     make(http.Header),
		Body:       http.NoBody,
		Request:    req,
	}, nil
}

// encodeOTLPJSON decodes one OTLP protobuf batch and re-encodes it as canonical
// OTLP-JSON. protojson emits compact output with no newlines, so one call is one
// line in the segment file.
//
// The OTLP/HTTP request body (Export*ServiceRequest) and the OTLP-JSON file
// wrapper (TracesData/LogsData/MetricsData) both carry the batch as field 1
// (repeated Resource*), so the request bytes decode directly into the file
// wrapper. Importing the signal-only proto packages (rather than the collector
// request packages) keeps gRPC out of the dependency graph (D14).
func encodeOTLPJSON(signal string, body []byte) ([]byte, error) {
	switch signal {
	case signalTraces:
		var data tracepb.TracesData
		if err := proto.Unmarshal(body, &data); err != nil {
			return nil, fmt.Errorf("decode trace batch: %w", err)
		}
		return protojson.Marshal(&data)
	case signalLogs:
		var data logspb.LogsData
		if err := proto.Unmarshal(body, &data); err != nil {
			return nil, fmt.Errorf("decode log batch: %w", err)
		}
		return protojson.Marshal(&data)
	case signalMetrics:
		var data metricspb.MetricsData
		if err := proto.Unmarshal(body, &data); err != nil {
			return nil, fmt.Errorf("decode metric batch: %w", err)
		}
		return protojson.Marshal(&data)
	default:
		return nil, fmt.Errorf("unknown OTLP signal %q", signal)
	}
}

func fileHTTPClient(signal string, w *segmentWriter) *http.Client {
	return &http.Client{Transport: &fileTransport{signal: signal, writer: w}}
}

// newFileTraceExporter builds the local trace exporter over the segment writer.
func newFileTraceExporter(ctx context.Context, w *segmentWriter) (*otlptrace.Exporter, error) {
	client := otlptracehttp.NewClient(
		otlptracehttp.WithEndpoint(localEndpointHost),
		otlptracehttp.WithInsecure(),
		otlptracehttp.WithHTTPClient(fileHTTPClient(signalTraces, w)),
	)
	return otlptrace.New(ctx, client)
}

// newFileMetricExporter builds the local metric exporter over the segment writer.
func newFileMetricExporter(ctx context.Context, w *segmentWriter) (sdkmetric.Exporter, error) {
	return otlpmetrichttp.New(ctx,
		otlpmetrichttp.WithEndpoint(localEndpointHost),
		otlpmetrichttp.WithInsecure(),
		otlpmetrichttp.WithHTTPClient(fileHTTPClient(signalMetrics, w)),
	)
}

// newFileLogExporter builds the local log exporter over the segment writer.
func newFileLogExporter(ctx context.Context, w *segmentWriter) (sdklog.Exporter, error) {
	return otlploghttp.New(ctx,
		otlploghttp.WithEndpoint(localEndpointHost),
		otlploghttp.WithInsecure(),
		otlploghttp.WithHTTPClient(fileHTTPClient(signalLogs, w)),
	)
}

// newNetworkTraceExporter builds the OTLP/HTTP trace exporter for a collector.
func newNetworkTraceExporter(ctx context.Context, cfg endpointConfig) (*otlptrace.Exporter, error) {
	opts, err := traceHTTPOptions(cfg)
	if err != nil {
		return nil, err
	}
	return otlptracehttp.New(ctx, opts...)
}

// newNetworkMetricExporter builds the OTLP/HTTP metric exporter for a collector.
func newNetworkMetricExporter(ctx context.Context, cfg endpointConfig) (sdkmetric.Exporter, error) {
	opts, err := metricHTTPOptions(cfg)
	if err != nil {
		return nil, err
	}
	return otlpmetrichttp.New(ctx, opts...)
}

// newNetworkLogExporter builds the OTLP/HTTP log exporter for a collector.
func newNetworkLogExporter(ctx context.Context, cfg endpointConfig) (sdklog.Exporter, error) {
	opts, err := logHTTPOptions(cfg)
	if err != nil {
		return nil, err
	}
	return otlploghttp.New(ctx, opts...)
}

func traceHTTPOptions(cfg endpointConfig) ([]otlptracehttp.Option, error) {
	tlsCfg, hasTLS, err := clientTLSConfig(cfg.caFile)
	if err != nil {
		return nil, err
	}
	host, insecure, err := endpointHost(cfg.endpoint, cfg.insecure)
	if err != nil {
		return nil, err
	}
	opts := endpointOptions(host, insecure, otlptracehttp.WithEndpoint, otlptracehttp.WithInsecure)
	if len(cfg.headers) > 0 {
		opts = append(opts, otlptracehttp.WithHeaders(cfg.headers))
	}
	if cfg.timeout > 0 {
		opts = append(opts, otlptracehttp.WithTimeout(cfg.timeout))
	}
	if hasTLS {
		opts = append(opts, otlptracehttp.WithTLSClientConfig(tlsCfg))
	}
	return opts, nil
}

func metricHTTPOptions(cfg endpointConfig) ([]otlpmetrichttp.Option, error) {
	tlsCfg, hasTLS, err := clientTLSConfig(cfg.caFile)
	if err != nil {
		return nil, err
	}
	host, insecure, err := endpointHost(cfg.endpoint, cfg.insecure)
	if err != nil {
		return nil, err
	}
	opts := endpointOptions(host, insecure, otlpmetrichttp.WithEndpoint, otlpmetrichttp.WithInsecure)
	if len(cfg.headers) > 0 {
		opts = append(opts, otlpmetrichttp.WithHeaders(cfg.headers))
	}
	if cfg.timeout > 0 {
		opts = append(opts, otlpmetrichttp.WithTimeout(cfg.timeout))
	}
	if hasTLS {
		opts = append(opts, otlpmetrichttp.WithTLSClientConfig(tlsCfg))
	}
	return opts, nil
}

func logHTTPOptions(cfg endpointConfig) ([]otlploghttp.Option, error) {
	tlsCfg, hasTLS, err := clientTLSConfig(cfg.caFile)
	if err != nil {
		return nil, err
	}
	host, insecure, err := endpointHost(cfg.endpoint, cfg.insecure)
	if err != nil {
		return nil, err
	}
	opts := endpointOptions(host, insecure, otlploghttp.WithEndpoint, otlploghttp.WithInsecure)
	if len(cfg.headers) > 0 {
		opts = append(opts, otlploghttp.WithHeaders(cfg.headers))
	}
	if cfg.timeout > 0 {
		opts = append(opts, otlploghttp.WithTimeout(cfg.timeout))
	}
	if hasTLS {
		opts = append(opts, otlploghttp.WithTLSClientConfig(tlsCfg))
	}
	return opts, nil
}

// endpointHost reduces the configured endpoint to a host[:port] and decides
// whether the transport is insecure. Using WithEndpoint (rather than
// WithEndpointURL) keeps the signal path appended by every exporter, so the
// configured value is a base URL as the spec documents.
func endpointHost(endpoint string, tlsInsecure bool) (string, bool, error) {
	if !strings.Contains(endpoint, "://") {
		return endpoint, tlsInsecure, nil
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", false, fmt.Errorf("telemetry endpoint: %w", err)
	}
	return u.Host, u.Scheme != "https" || tlsInsecure, nil
}

// endpointOptions selects a host endpoint and marks it insecure when required.
func endpointOptions[T any](host string, insecure bool, withEndpoint func(string) T, withInsecure func() T) []T {
	opts := []T{withEndpoint(host)}
	if insecure {
		opts = append(opts, withInsecure())
	}
	return opts
}

// clientTLSConfig builds a TLS config with a custom CA when one is configured.
// The boolean reports whether a config was produced (an empty CA path yields no
// config, not a nil one).
func clientTLSConfig(caFile string) (*tls.Config, bool, error) {
	if caFile == "" {
		return nil, false, nil
	}
	pem, err := os.ReadFile(filepath.Clean(caFile))
	if err != nil {
		return nil, false, fmt.Errorf("read CA file: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, false, fmt.Errorf("CA file %s contains no certificates", caFile)
	}
	return &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}, true, nil
}
