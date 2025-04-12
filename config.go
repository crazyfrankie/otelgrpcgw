package otelgrpcgw

import (
	"context"
	"net/http"
	"net/http/httptrace"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

type config struct {
	ServerName         string
	Tracer             trace.Tracer                                 // Tracer instance used to create the span
	Meter              metric.Meter                                 // Meter instances for generating metrics
	Propagators        propagation.TextMapPropagator                // Context propagator for passing trace context across services
	SpanStartOptions   []trace.SpanStartOption                      // Additional options when creating a span, such as setting attributes, links.
	PublicEndpoint     bool                                         // Whether it is the start of the link (without extracting the trace context from the request header)
	PublicEndpointFn   func(*http.Request) bool                     // Dynamically determine if it is a PublicEndpoint
	ReadEvent          bool                                         // Whether to log events that read the request body
	WriteEvent         bool                                         // Whether to log events written to the response body
	Filters            []Filter                                     // request filter, return false to indicate that the request is not logged trace
	MetricAttributesFn func(*http.Request) []attribute.KeyValue     // Label generation functions for custom metrics, e.g., add labels based on paths, status codes
	ClientTrace        func(context.Context) *httptrace.ClientTrace // Create ClientTrace to trace downstream HTTP requests (connection, DNS, TTFB, etc.)
	SpanNameFormatter  func(string, *http.Request) string
	TracerProvider     trace.TracerProvider
	MeterProvider      metric.MeterProvider
}

type Option func(*config)

func newConfig(opts ...Option) *config {
	c := &config{
		Propagators:   otel.GetTextMapPropagator(),
		MeterProvider: otel.GetMeterProvider(),
	}
	for _, opt := range opts {
		opt(c)
	}

	if c.TracerProvider != nil {
		c.Tracer = newTracer(c.TracerProvider)
	}

	c.Meter = newMeter(c.MeterProvider)

	return c
}

// WithTracerProvider specifies a tracer provider to use for creating a tracer.
// If none is specified, the global provider is used.
func WithTracerProvider(tp trace.TracerProvider) Option {
	return func(c *config) {
		c.TracerProvider = tp
	}
}

// WithMeterProvider specifies a meter provider to use for creating a meter.
// If none is specified, the global provider is used.
func WithMeterProvider(mp metric.MeterProvider) Option {
	return func(c *config) {
		c.MeterProvider = mp
	}
}

// WithPublicEndpoint sets PublicEndpoint to true.
// PublicEndpoint indicates whether the service is the starting point of the link.
//
// If true, the current service is the starting point of the link, a new root span is created,
// and the trace context from upstream is recorded via Link (but not as a child span).
//
// If false (the default), the current span will act as a child of the upstream trace context.
//
// For gateways, portal services, etc.
func WithPublicEndpoint() Option {
	return func(c *config) {
		c.PublicEndpoint = true
	}
}

// WithPublicEndpointFn sets the value of PublicEndpointFn.
// PublicEndpointFn is used to dynamically determine if a request is a link origin.
// When true, the behavior is the same as PublicEndpoint=true, for more granular control over the processing logic.
func WithPublicEndpointFn(fn func(r *http.Request) bool) Option {
	return func(c *config) {
		c.PublicEndpointFn = fn
	}
}

// WithPropagators configures specific propagators.
// If this option isn't specified, then the global TextMapPropagator is used.
func WithPropagators(ps propagation.TextMapPropagator) Option {
	return func(c *config) {
		c.Propagators = ps
	}
}

// WithSpanOptions configures an additional set of trace.SpanStartOption,
// which are applied to each new span.
func WithSpanOptions(opts ...trace.SpanStartOption) Option {
	return func(c *config) {
		c.SpanStartOptions = append(c.SpanStartOptions, opts...)
	}
}

// WithFilter adds a filter to the list of filters used by the handler.
// If any filter indicates to exclude a request, then the request will not be traced.
// All filters must allow a request to be traced for a Span to be created.
// If no filters are provided, then all requests are traced.
func WithFilter(f Filter) Option {
	return func(c *config) {
		c.Filters = append(c.Filters, f)
	}
}

// WithSpanNameFormatter takes a function that will be called on every
// request, and the returned string will become the Span Name.
func WithSpanNameFormatter(fn func(operation string, r *http.Request) string) Option {
	return func(c *config) {
		c.SpanNameFormatter = fn
	}
}

// WithClientTrace takes a function that returns client trace instance that will be
// applied to the requests sent through the otelgrpcgw Transport.
func WithClientTrace(fn func(context.Context) *httptrace.ClientTrace) Option {
	return func(c *config) {
		c.ClientTrace = fn
	}
}

// WithServerName returns an Option that sets the name of the (virtual) server
// handling requests.
func WithServerName(server string) Option {
	return func(c *config) {
		c.ServerName = server
	}
}

// WithMetricAttributesFn returns an Option to set a function that maps an HTTP request to a slice of attribute.KeyValue.
// These attributes will be included in metrics for every request.
func WithMetricAttributesFn(metricAttributesFn func(r *http.Request) []attribute.KeyValue) Option {
	return func(c *config) {
		c.MetricAttributesFn = metricAttributesFn
	}
}
