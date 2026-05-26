package logging

import (
	"log/slog"
	"os"

	"camera-appliance/camera-manager/internal/redaction"
)

func New() *slog.Logger {
	return slog.New(redactingHandler{slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{})})
}

type redactingHandler struct {
	slog.Handler
}

func (h redactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return redactingHandler{h.Handler.WithAttrs(redactAttrs(attrs))}
}

func redactAttrs(attrs []slog.Attr) []slog.Attr {
	out := make([]slog.Attr, len(attrs))
	for i, attr := range attrs {
		if attr.Value.Kind() == slog.KindString {
			attr.Value = slog.StringValue(redaction.Text(attr.Value.String()))
		}
		out[i] = attr
	}
	return out
}
