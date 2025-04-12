package otelgrpcgw

import (
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

func newTracer(tp trace.TracerProvider) trace.Tracer {
	return tp.Tracer(ScopeName, trace.WithInstrumentationVersion(Version()))
}

func newMeter(mp metric.MeterProvider) metric.Meter {
	return mp.Meter(ScopeName, metric.WithInstrumentationVersion(Version()))
}
