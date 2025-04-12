package otelgrpcgw

import (
	"context"
	"time"
)

var startTimeKey struct{}

// ContextWithStartTime returns a context and puts start in it.
// Note: this can only be called once in the call chain,
// otherwise the previously set start will be overwritten and the measurement will not be accurate.
func ContextWithStartTime(parent context.Context, start time.Time) context.Context {
	return context.WithValue(parent, startTimeKey, start)
}

// StartTimeFromContext retrieves the start time from the given ctx,
// returns if it exists, or 0 if it does not.
func StartTimeFromContext(ctx context.Context) time.Time {
	t, _ := ctx.Value(startTimeKey).(time.Time)
	return t
}
