package observability

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"runtime"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

var (
	baseLogger *slog.Logger
	initOnce   sync.Once
)

// InitLogger initializes the global logger and sets it as the default.
func InitLogger() *slog.Logger {
	initOnce.Do(func() {
		jsonHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			AddSource: true,
			Level:     slog.LevelDebug,
		})
		logger := slog.New(NewAPMHandler(jsonHandler))
		slog.SetDefault(logger)
		baseLogger = logger
	})
	return baseLogger
}

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

func (l *Log) getCtx() context.Context {
	return l.obs.Context()
}

func (l *Log) log(level slog.Level, msg string, args ...any) {
	ctx := l.getCtx()
	if !l.logger.Enabled(ctx, level) {
		return
	}
	var pcs [1]uintptr
	runtime.Callers(3, pcs[:])
	r := slog.NewRecord(time.Now(), level, msg, pcs[0])
	r.Add(args...)
	_ = l.logger.Handler().Handle(ctx, r)
}

func (l *Log) Debug(msg string, args ...any) {
	l.log(slog.LevelDebug, msg, args...)
}

func (l *Log) Info(msg string, args ...any) {
	l.log(slog.LevelInfo, msg, args...)
}

func (l *Log) Warn(msg string, args ...any) {
	l.log(slog.LevelWarn, msg, args...)
}

func (l *Log) Error(msg string, args ...any) {
	l.log(slog.LevelError, msg, args...)
}

func (l *Log) With(args ...any) *Log {
	return &Log{
		obs:    l.obs,
		logger: l.logger.With(args...),
	}
}

// --- APMHandler for slog integration ---

type APMHandler struct {
	slog.Handler
	attrs []slog.Attr
}

func NewAPMHandler(baseHandler slog.Handler) *APMHandler {
	return &APMHandler{
		Handler: baseHandler,
	}
}

func (h *APMHandler) Handle(ctx context.Context, r slog.Record) error {
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return h.Handler.Handle(ctx, r)
	}

	if r.Level >= slog.LevelError {
		var loggedErr error
		r.Attrs(func(attr slog.Attr) bool {
			if attr.Key == "error" {
				if errVal, ok := attr.Value.Any().(error); ok {
					loggedErr = errVal
					return false
				}
			}
			return true
		})

		if loggedErr == nil {
			loggedErr = errors.New(r.Message)
		}

		span.RecordError(loggedErr)
		span.SetStatus(codes.Error, r.Message)
	} else {
		attrs := make([]attribute.KeyValue, 0, r.NumAttrs())
		r.Attrs(func(a slog.Attr) bool {
			attrs = append(attrs, attribute.String(a.Key, a.Value.String()))
			return true
		})
		span.AddEvent(r.Message, trace.WithAttributes(attrs...))
	}

	return h.Handler.Handle(ctx, r)
}

func (h *APMHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, 0, len(h.attrs)+len(attrs))
	newAttrs = append(newAttrs, h.attrs...)
	newAttrs = append(newAttrs, attrs...)

	return &APMHandler{
		Handler: h.Handler.WithAttrs(attrs),
		attrs:   newAttrs,
	}
}

func (h *APMHandler) WithGroup(name string) slog.Handler {
	return NewAPMHandler(h.Handler.WithGroup(name))
}

func (h *APMHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.Handler.Enabled(ctx, level)
}
