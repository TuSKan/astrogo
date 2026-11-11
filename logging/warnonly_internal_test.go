package logging

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

// A logger derived from the default with With or WithGroup keeps the default's
// filter: progress dropped, warnings kept, and the attributes and group carried.
//
// The default handler wraps a text handler to drop Info without formatting it.
// WithAttrs and WithGroup return a new wrapper rather than the inner handler, so
// logging.Logger().With(...) is still the default; returning the inner handler
// alone would keep the level by coincidence of its options and lose the cheap
// path.
func TestTheDefaultHandlerKeepsItsFilterThroughWithAttrsAndWithGroup(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	base := &warnOnlyHandler{inner: slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})}

	for name, h := range map[string]slog.Handler{
		"WithAttrs": base.WithAttrs([]slog.Attr{slog.String("body", "moon")}),
		"WithGroup": base.WithGroup("eop"),
	} {
		buf.Reset()

		if _, ok := h.(*warnOnlyHandler); !ok {
			t.Errorf("%s returned a %T, not the default's own handler", name, h)
		}

		l := slog.New(h)

		if l.Enabled(t.Context(), slog.LevelInfo) {
			t.Errorf("%s: a derived default logger passes Info", name)
		}

		l.Info("progress")
		l.Warn("degraded", "mjd", 60000)

		out := buf.String()

		if strings.Contains(out, "progress") {
			t.Errorf("%s: Info reached the output:\n%s", name, out)
		}

		if !strings.Contains(out, "degraded") {
			t.Errorf("%s: Warn did not reach the output:\n%s", name, out)
		}

		want := map[string]string{"WithAttrs": "body=moon", "WithGroup": "eop.mjd=60000"}[name]
		if !strings.Contains(out, want) {
			t.Errorf("%s: the output does not carry %q:\n%s", name, want, out)
		}
	}
}
