package otelgrpcgw

import (
	"net/http"
	"time"

	"github.com/felixge/httpsnoop"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/crazyfrankie/otelgrpcgw/internal/request"
	"github.com/crazyfrankie/otelgrpcgw/internal/semconv"
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
	reqStartTime := time.Now()
	// filters
	for _, f := range m.filters {
		if !f(r) {
			// Reject — short-circuit here
			return
		}
	}

	// extract ctx
	ctx := m.propagators.Extract(r.Context(), propagation.HeaderCarrier(r.Header))
	opts := []trace.SpanStartOption{
		trace.WithAttributes(m.semconv.RequestTraceAttrs(m.server, r, semconv.RequestTraceAttrsOpts{})...),
	}

	if m.publicEndpoint || (m.publicEndpointFn != nil && m.publicEndpointFn(r.WithContext(ctx))) {
		opts = append(opts, trace.WithNewRoot())
		if s := trace.SpanContextFromContext(ctx); s.IsValid() && s.IsRemote() {
			opts = append(opts, trace.WithLinks(trace.Link{SpanContext: s}))
		}
	}

	tracer := m.tracer
	if tracer == nil {
		if span := trace.SpanFromContext(r.Context()); span.SpanContext().IsValid() {
			tracer = newTracer(span.TracerProvider())
		} else {
			tracer = newTracer(otel.GetTracerProvider())
		}
	}

	if startTime := StartTimeFromContext(ctx); !startTime.IsZero() {
		opts = append(opts, trace.WithTimestamp(startTime))
		reqStartTime = startTime
	}

	ctx, span := tracer.Start(ctx, m.spanNameFormatter(m.operation, r), opts...)
	defer span.End()

	readRecordFunc := func(int64) {}
	if m.readEvent {
		readRecordFunc = func(n int64) {
			span.AddEvent("read", trace.WithAttributes(ReadBytesKey.Int64(n)))
		}
	}

	bw := request.NewBodyWrapper(r.Body, readRecordFunc)
	if r.Body != nil && r.Body != http.NoBody {
		r.Body = bw
	}

	writeRecordFunc := func(int64) {}
	if m.writeEvent {
		writeRecordFunc = func(n int64) {
			span.AddEvent("write", trace.WithAttributes(WroteBytesKey.Int64(n)))
		}
	}

	rww := request.NewRespWriterWrapper(w, writeRecordFunc)

	// wrap http.ResponseWriter
	w = httpsnoop.Wrap(w, httpsnoop.Hooks{
		Header: func(httpsnoop.HeaderFunc) httpsnoop.HeaderFunc {
			return rww.Header
		},
		Write: func(httpsnoop.WriteFunc) httpsnoop.WriteFunc {
			return rww.Write
		},
		WriteHeader: func(httpsnoop.WriteHeaderFunc) httpsnoop.WriteHeaderFunc {
			return rww.WriteHeader
		},
		Flush: func(httpsnoop.FlushFunc) httpsnoop.FlushFunc {
			return rww.Flush
		},
	})

	labeler, found := LabelerFromContext(ctx)
	if !found {
		ctx = ContextWithLabeler(ctx, labeler)
	}

	next(w, r.WithContext(ctx), pathParams)

	// collect metrics
	statusCode := rww.StatusCode()
	bytesWritten := rww.BytesWritten()
	span.SetStatus(m.semconv.Status(statusCode))
	span.SetAttributes(m.semconv.ResponseTraceAttrs(semconv.ResponseTelemetry{
		StatusCode: statusCode,
		ReadBytes:  bw.BytesRead(),
		ReadError:  bw.Error(),
		WriteBytes: bytesWritten,
		WriteError: rww.Error(),
	})...)

	elapsedTime := float64(time.Since(reqStartTime)) / float64(time.Millisecond)
	metricAttributes := semconv.MetricAttributes{
		Req:                  r,
		StatusCode:           statusCode,
		AdditionalAttributes: append(labeler.Get(), m.metricAttributesFromRequest(r)...),
	}

	m.semconv.RecordMetrics(ctx, semconv.ServerMetricData{
		ServerName:       m.server,
		ResponseSize:     bytesWritten,
		MetricAttributes: metricAttributes,
		MetricData: semconv.MetricData{
			RequestSize: bw.BytesRead(),
			ElapsedTime: elapsedTime,
		},
	})
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

func (m *handler) metricAttributesFromRequest(r *http.Request) []attribute.KeyValue {
	var attributeForRequest []attribute.KeyValue
	if m.metricAttributesFn != nil {
		attributeForRequest = m.metricAttributesFn(r)
	}
	return attributeForRequest
}
