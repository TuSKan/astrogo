package logging_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/logging"
)

// Every entry point records the caller's line as the source, not this package.
//
// TestSourceIsTheCallerNotThisPackage holds Info to that; the other three share
// emit's frame count, so a wrapper added between any of them and emit would move
// their records into logging.go while Info's stayed right.
func TestEveryEntryPointRecordsItsCallerAsTheSource(t *testing.T) {
	// Not parallel: it swaps the process-wide logger.
	defer logging.Set(nil)

	ctx := t.Context()

	for name, log := range map[string]func(){
		"Info":        func() { logging.Info("probe") },
		"Warn":        func() { logging.Warn("probe") },
		"InfoContext": func() { logging.InfoContext(ctx, "probe") },
		"WarnContext": func() { logging.WarnContext(ctx, "probe") },
	} {
		var buf bytes.Buffer

		logging.Set(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{
			Level:     slog.LevelInfo,
			AddSource: true,
		})))

		log()

		out := buf.String()
		if !strings.Contains(out, "entrypoints_test.go") {
			t.Errorf("%s: the record's source is not this test file:\n%s", name, out)
		}
	}
}

// ctxKey carries a value through a context to the handler.
type ctxKey struct{}

// recorder is a handler that keeps what it was handed.
type recorder struct {
	levels []slog.Level
	values []any
}

func (r *recorder) Enabled(context.Context, slog.Level) bool { return true }

func (r *recorder) Handle(ctx context.Context, rec slog.Record) error {
	r.levels = append(r.levels, rec.Level)
	r.values = append(r.values, ctx.Value(ctxKey{}))

	return nil
}

func (r *recorder) WithAttrs([]slog.Attr) slog.Handler { return r }
func (r *recorder) WithGroup(string) slog.Handler      { return r }

// The context variants hand their context to the handler, at their own level.
//
// That is their whole reason to exist: a handler reads trace identifiers from
// the context, and a variant that passed context.Background instead would log
// the line with none, and nothing would fail.
func TestContextVariantsPassTheirContextToTheHandler(t *testing.T) {
	defer logging.Set(nil)

	rec := &recorder{}
	logging.Set(slog.New(rec))

	ctx := context.WithValue(t.Context(), ctxKey{}, "trace-42")

	logging.InfoContext(ctx, "progress")
	logging.WarnContext(ctx, "degraded")

	if len(rec.levels) != 2 {
		t.Fatalf("the handler saw %d records, want 2", len(rec.levels))
	}

	for i, want := range []slog.Level{slog.LevelInfo, slog.LevelWarn} {
		if rec.levels[i] != want {
			t.Errorf("record %d is at %v, want %v", i, rec.levels[i], want)
		}

		if rec.values[i] != "trace-42" {
			t.Errorf("record %d reached the handler with context value %v; the caller's "+
				"context was not passed on", i, rec.values[i])
		}
	}
}
