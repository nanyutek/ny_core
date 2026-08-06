package nylog

import (
	"context"
	"fmt"
	"log/slog"
	"os"
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
type WriteErrorHandler func(err error, r slog.Record)

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

// TeeHandler 实现 slog.Handler 接口，支持多后端双写分流、Context 动态属性提取及错误降级兜底
type TeeHandler struct {
	handlers          []slog.Handler
	contextExtractors []ContextExtractor
	writeErrorHandler WriteErrorHandler
}

func NewTeeHandler(handlers ...slog.Handler) *TeeHandler {
	// 空 Handler 场景保护：若没有传入任何有效 Handler，至少输出控制台兜底，防止静默丢日志
	if len(handlers) == 0 {
		fmt.Fprintln(os.Stderr, "[nylog WARNING] No valid handlers configured. Falling back to os.Stderr JSON output.")
		handlers = []slog.Handler{slog.NewJSONHandler(os.Stderr, nil)}
	}

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

func (h *TeeHandler) WithErrorHandler(fn WriteErrorHandler) *TeeHandler {
	h2 := *h
	h2.writeErrorHandler = fn
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
		// 1. 从 Context 提取通用绑定的 Context 动态属性
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

	// 3. 多 Handler 扇出分流与写入异常捕获
	var writeErr error
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, r.Level) {
			if err := handler.Handle(ctx, r.Clone()); err != nil {
				writeErr = err
				if h.writeErrorHandler != nil {
					h.writeErrorHandler(err, r)
				} else {
					// 默认落盘失败降级输出到 os.Stderr，防止问题被静默吞掉
					fmt.Fprintf(os.Stderr, "[nylog ERROR] Log handler write failed: %v | msg: %s\n", err, r.Message)
				}
			}
		}
	}
	return writeErr
}

func (h *TeeHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newHandlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		newHandlers[i] = handler.WithAttrs(attrs)
	}
	return &TeeHandler{
		handlers:          newHandlers,
		contextExtractors: h.contextExtractors,
		writeErrorHandler: h.writeErrorHandler,
	}
}

func (h *TeeHandler) WithGroup(name string) slog.Handler {
	newHandlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		newHandlers[i] = handler.WithGroup(name)
	}
	return &TeeHandler{
		handlers:          newHandlers,
		contextExtractors: h.contextExtractors,
		writeErrorHandler: h.writeErrorHandler,
	}
}
