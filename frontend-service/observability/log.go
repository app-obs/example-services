
package observability

import (
	"context"
	"errors"
	"log/slog"
	"runtime"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Log wraps the slog logger.
type Log struct {
	obs    *Observability
	logger *slog.Logger
}

// NewLog creates a new Log instance.
func NewLog(obs *Observability, baseLogger *slog.Logger) *Log {
	return &Log{
		obs:    obs,
		logger: baseLogger,
	}
}

// getCtx returns the context from the Observability instance.
func (l *Log) getCtx() context.Context {
	return l.obs.Context()
}

// log is a helper to handle logging calls, setting the correct caller source.
func (l *Log) log(level slog.Level, msg string, args ...any) {
	ctx := l.getCtx()
	if !l.logger.Enabled(ctx, level) {
		return
	}
	var pcs [1]uintptr
	runtime.Callers(3, pcs[:]) // skip [runtime.Callers, log, Info/Debug/...]
	r := slog.NewRecord(time.Now(), level, msg, pcs[0])
	r.Add(args...)
	_ = l.logger.Handler().Handle(ctx, r)
}

// Debug logs at Debug level.
func (l *Log) Debug(msg string, args ...any) {
	l.log(slog.LevelDebug, msg, args...)
}

// Info logs at Info level.
func (l *Log) Info(msg string, args ...any) {
	l.log(slog.LevelInfo, msg, args...)
}

// Warn logs at Warn level.
func (l *Log) Warn(msg string, args ...any) {
	l.log(slog.LevelWarn, msg, args...)
}

// Error logs at Error level.
func (l *Log) Error(msg string, args ...any) {
	l.log(slog.LevelError, msg, args...)
}

// With creates a new Log instance with additional attributes.
func (l *Log) With(args ...any) *Log {
	return &Log{
		obs:    l.obs,
		logger: l.logger.With(args...),
	}
}
// --- APMHandler for slog integration with OpenTelemetry ---

// APMHandler is a custom slog.Handler that adds OpenTelemetry trace/span IDs
// and records errors on the active span.
type APMHandler struct {
	slog.Handler
	attrs []slog.Attr // Accumulated attributes from WithAttrs
}

// NewAPMHandler creates a new APMHandler that wraps a base slog.Handler.
func NewAPMHandler(baseHandler slog.Handler) *APMHandler {
	return &APMHandler{
		Handler: baseHandler,
	}
}

// Handle implements slog.Handler.Handle. It adds trace/span IDs and records errors
// on the active OpenTelemetry span if one is present in the context.
func (h *APMHandler) Handle(ctx context.Context, r slog.Record) error {
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() {
		spanCtx := span.SpanContext()
		if spanCtx.HasTraceID() {
			r.AddAttrs(slog.String("trace.id", spanCtx.TraceID().String()))
		}
		if spanCtx.HasSpanID() {
			r.AddAttrs(slog.String("span.id", spanCtx.SpanID().String()))
		}

		// Collect ALL attributes: those from logger.With AND those from the current log call
		allAttrs := make([]slog.Attr, 0, len(h.attrs)+r.NumAttrs())
		allAttrs = append(allAttrs, h.attrs...) // 1. Start with handler's accumulated WithAttrs

		// 2. Add attributes from the current slog.Record itself
		r.Attrs(func(attr slog.Attr) bool {
			allAttrs = append(allAttrs, attr)
			return true
		})

		// If it's an error level, record the error on the span
		if r.Level == slog.LevelError {
			var loggedErr error
			r.Attrs(func(attr slog.Attr) bool {
				if attr.Key == "error" {
					if errVal, ok := attr.Value.Any().(error); ok {
						loggedErr = errVal
						return false // Stop iterating
					}
				}
				return true
			})

			// Convert all attributes for the span event.
			apmAttrs := convertSlogAttrsToAPMAttrsFromSlice(allAttrs)

			if loggedErr != nil {
				// Add standard error event attributes.
				apmAttrs = append(apmAttrs, attribute.String("event", "log_error"), attribute.String("message", r.Message))
				span.RecordError(loggedErr, trace.WithAttributes(apmAttrs...))
				span.SetStatus(codes.Error, loggedErr.Error())
			} else {
				// Record the log message itself as an error if no specific error object.
				apmAttrs = append(apmAttrs, attribute.String("event", "log_error"))
				span.RecordError(errors.New(r.Message), trace.WithAttributes(apmAttrs...))
				span.SetStatus(codes.Error, r.Message)
			}
		} else if r.Level == slog.LevelInfo || r.Level == slog.LevelWarn {
			// Convert slog attributes to OpenTelemetry attributes
			apmAttrs := convertSlogAttrsToAPMAttrsFromSlice(allAttrs)
			// Add an event to the OpenTelemetry span with the record's message and converted attributes
			span.AddEvent(r.Message, trace.WithAttributes(apmAttrs...))
		}
	}

	// Always call the base handler to actually output the log record
	return h.Handler.Handle(ctx, r)
}

// WithAttrs implements slog.Handler.WithAttrs. It returns a new APMHandler
// with the combined attributes.
func (h *APMHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, 0, len(h.attrs)+len(attrs))
	newAttrs = append(newAttrs, h.attrs...)   // Include existing attrs
	newAttrs = append(newAttrs, attrs...)     // Add new attrs

	return &APMHandler{
		Handler: h.Handler.WithAttrs(attrs), // Still pass new attrs to the wrapped handler
		attrs:   newAttrs,                   // Store the combined attrs in the new APMHandler
	}
}

// WithGroup implements slog.Handler.WithGroup. It returns a new APMHandler
// with the new group applied to the underlying handler.
func (h *APMHandler) WithGroup(name string) slog.Handler {
	return NewAPMHandler(h.Handler.WithGroup(name))
}

// Enabled implements slog.Handler.Enabled.
func (h *APMHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.Handler.Enabled(ctx, level)
}

// convertSlogAttrsToAPMAttrsFromSlice converts a slice of slog.Attr to OpenTelemetry attribute.KeyValue.
func convertSlogAttrsToAPMAttrsFromSlice(slogAttrs []slog.Attr) []attribute.KeyValue {
	spanAttributes := make([]attribute.KeyValue, 0, len(slogAttrs))
	for _, attr := range slogAttrs {
		spanAttributes = append(spanAttributes, attribute.String(attr.Key, attr.Value.String()))
	}
	return spanAttributes
}
