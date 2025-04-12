package otelgrpcgw

import (
	"github.com/crazyfrankie/otelgrpcgw/internal/semconv"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"net/http"
)

// Filter is a predicate used to determine whether a given http.request should
// be traced. A Filter must return true if the request should be traced.
type Filter func(*http.Request) bool

// ScopeName is the instrumentation scope name.
const ScopeName = "github.com/crazyfrankie/otelgrpcgw"

type handler struct {
	operation string
	server    string

	tracer             trace.Tracer
	propagators        propagation.TextMapPropagator
	spanStartOptions   []trace.SpanStartOption
	readEvent          bool
	writeEvent         bool
	filters            []Filter
	spanNameFormatter  func(string, *http.Request) string
	publicEndpoint     bool
	publicEndpointFn   func(*http.Request) bool
	metricAttributesFn func(*http.Request) []attribute.KeyValue
	semconv            semconv.HTTPServer
}

func defaultHandlerFormatter(operation string, _ *http.Request) string {
	return operation
}

func NewHandler(handler runtime.HandlerFunc, operation string, opts ...Option) runtime.HandlerFunc {
	return NewMiddleware(operation, opts...)(handler)
}

func NewMiddleware(operation string, opts ...Option) runtime.Middleware {
	h := handler{
		operation: operation,
	}

	defaultOpts := []Option{
		WithSpanOptions(trace.WithSpanKind(trace.SpanKindServer)),
		WithSpanNameFormatter(defaultHandlerFormatter),
	}

	cfg := newConfig(append(defaultOpts, opts...)...)
	h.configure(cfg)

	return func(next runtime.HandlerFunc) runtime.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request, pathParams map[string]string) {
			h.serveHTTP(w, r, next, pathParams)
		}
	}
}

// serveHTTP sets up tracing and calls the given next runtime.HandlerFunc with the span
// context injected into the request context.
func (m *handler) serveHTTP(w http.ResponseWriter, r *http.Request, next runtime.HandlerFunc, pathParams map[string]string) {
	//startTime := time.Now()
	for _, f := range m.filters {
		if !f(r) {
			// TODO
		}
	}
}

// configure executes the configuration from config into the handler.
func (m *handler) configure(c *config) {
	m.tracer = c.Tracer
	m.propagators = c.Propagators
	m.spanStartOptions = c.SpanStartOptions
	m.readEvent = c.ReadEvent
	m.writeEvent = c.WriteEvent
	m.filters = c.Filters
	m.spanNameFormatter = c.SpanNameFormatter
	m.publicEndpoint = c.PublicEndpoint
	m.publicEndpointFn = c.PublicEndpointFn
	m.server = c.ServerName
	m.semconv = semconv.NewHTTPServer(c.Meter)
	m.metricAttributesFn = c.MetricAttributesFn
}
