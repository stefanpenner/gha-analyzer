package githubapi

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/binary"
	"fmt"

	"go.opentelemetry.io/otel/trace"
)

type contextKey string

const (
	traceIDKey contextKey = "gha-trace-id"
	spanIDKey  contextKey = "gha-span-id"
)

// NewTraceID returns a deterministic TraceID based on GitHub Workflow Run ID and Attempt.
func NewTraceID(runID int64, runAttempt int64) trace.TraceID {
	if runAttempt == 0 {
		runAttempt = 1
	}
	id := fmt.Sprintf("%d-%d", runID, runAttempt)
	return trace.TraceID(md5.Sum([]byte(id)))
}

// NewSpanID returns a deterministic SpanID based on a GitHub ID (e.g., Job ID).
func NewSpanID(id int64) trace.SpanID {
	var sid trace.SpanID
	binary.BigEndian.PutUint64(sid[:], uint64(id))
	return sid
}

// NewSpanIDFromString returns a deterministic SpanID based on a string (e.g., Step Name).
func NewSpanIDFromString(s string) trace.SpanID {
	sum := md5.Sum([]byte(s))
	var sid trace.SpanID
	copy(sid[:], sum[:8])
	return sid
}

// ContextWithIDs returns a new context with the given TraceID and SpanID hints for the GHIDGenerator.
func ContextWithIDs(ctx context.Context, tid trace.TraceID, sid trace.SpanID) context.Context {
	if tid.IsValid() {
		ctx = context.WithValue(ctx, traceIDKey, tid)
	}
	if sid.IsValid() {
		ctx = context.WithValue(ctx, spanIDKey, sid)
	}
	return ctx
}

// GHIDGenerator is an OpenTelemetry IDGenerator that uses IDs from the context if available.
type GHIDGenerator struct{}

func (g GHIDGenerator) NewIDs(ctx context.Context) (trace.TraceID, trace.SpanID) {
	tid, _ := ctx.Value(traceIDKey).(trace.TraceID)
	sid, _ := ctx.Value(spanIDKey).(trace.SpanID)

	if !tid.IsValid() {
		_, _ = rand.Read(tid[:])
	}
	if !sid.IsValid() {
		_, _ = rand.Read(sid[:])
	}

	return tid, sid
}

func (g GHIDGenerator) NewSpanID(ctx context.Context, traceID trace.TraceID) trace.SpanID {
	sid, _ := ctx.Value(spanIDKey).(trace.SpanID)
	if !sid.IsValid() {
		_, _ = rand.Read(sid[:])
	}
	return sid
}
