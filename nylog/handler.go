package nylog

import (
	"context"
	"log/slog"
)

type ctxKey string

const slogContextAttrsKey ctxKey = "slog_context_attrs"

// WithContextAttr 允许将动态的 slog.Attr 绑进 Context (如在 HTTP 中间件中塞入 request_id / user_id)
func WithContextAttr(parent context.Context, attrs ...slog.Attr) context.Context {
	if parent == nil {
		parent = context.Background()
	}
	existing, _ := parent.Value(slogContextAttrsKey).([]slog.Attr)
	newAttrs := append(append([]slog.Attr{}, existing...), attrs...)
	return context.WithValue(parent, slogContextAttrsKey, newAttrs)
}

type ContextExtractor func(ctx context.Context) []slog.Attr

// defaultTraceExtractor 默认提取器：未自定义时，自动从 Context 提取 "trace_id" 或 "request_id"
func defaultTraceExtractor(ctx context.Context) []slog.Attr {
	if ctx == nil {
		return nil
	}
	var attrs []slog.Attr
	if traceID, ok := ctx.Value("trace_id").(string); ok && traceID != "" {
		attrs = append(attrs, slog.String("trace_id", traceID))
	}
	if reqID, ok := ctx.Value("request_id").(string); ok && reqID != "" {
		attrs = append(attrs, slog.String("request_id", reqID))
	}
	return attrs
}

// TeeHandler 实现 slog.Handler 接口，支持多后端双写分流与 Context 属性自动提取
type TeeHandler struct {
	handlers          []slog.Handler
	contextExtractors []ContextExtractor
}

func NewTeeHandler(handlers ...slog.Handler) *TeeHandler {
	return &TeeHandler{
		handlers:          handlers,
		contextExtractors: []ContextExtractor{defaultTraceExtractor},
	}
}

func (h *TeeHandler) WithExtractors(extractors ...ContextExtractor) *TeeHandler {
	if len(extractors) == 0 {
		return h
	}
	h2 := *h
	h2.contextExtractors = append(append([]ContextExtractor{}, h.contextExtractors...), extractors...)
	return &h2
}

func (h *TeeHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (h *TeeHandler) Handle(ctx context.Context, r slog.Record) error {
	if ctx != nil {
		// 1. 从 Context 提取通用绑定的 Context 动态属性 (如通过 WithContextAttr 绑定的属性)
		if attrs, ok := ctx.Value(slogContextAttrsKey).([]slog.Attr); ok && len(attrs) > 0 {
			r.AddAttrs(attrs...)
		}
		// 2. 执行所有的提取器 (包含内置默认逻辑或用户自定义的 TraceKey/Extractor 提取器)
		for _, extractor := range h.contextExtractors {
			if attrs := extractor(ctx); len(attrs) > 0 {
				r.AddAttrs(attrs...)
			}
		}
	}

	// 3. 多 Handler 扇出分流
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, r.Level) {
			_ = handler.Handle(ctx, r.Clone())
		}
	}
	return nil
}

func (h *TeeHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newHandlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		newHandlers[i] = handler.WithAttrs(attrs)
	}
	return &TeeHandler{handlers: newHandlers, contextExtractors: h.contextExtractors}
}

func (h *TeeHandler) WithGroup(name string) slog.Handler {
	newHandlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		newHandlers[i] = handler.WithGroup(name)
	}
	return &TeeHandler{handlers: newHandlers, contextExtractors: h.contextExtractors}
}
