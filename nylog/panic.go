package nylog

import (
	"context"
	"log/slog"
	"runtime/debug"
)

// ErrorAttrs 包含堆栈信息的强类型 Error 日志
func (l *Logger) ErrorAttrs(ctx context.Context, msg string, err error, attrs ...slog.Attr) {
	if err != nil {
		attrs = append(attrs, Err(err))
	}
	if l.conf.PrintErrorStack {
		attrs = append(attrs, slog.String("stack", string(debug.Stack())))
	}
	l.LogAttrs(ctx, slog.LevelError, msg, attrs...)
}

// CatchPanic 崩溃拦截器组件
func CatchPanic(ctx context.Context) {
	if r := recover(); r != nil {
		Get().ErrorAttrs(ctx, "[SYSTEM PANIC INTERCEPTED]", nil,
			slog.Any("panic_error", r),
			slog.String("stack", string(debug.Stack())),
		)
	}
}

// SetLevel 动态切换全局日志级别
func SetLevel(level string) {
	Get().levelVar.Set(parseLevel(level))
}

// ErrorAttrs 包级代理：包含堆栈信息的强类型 Error 日志
func ErrorAttrs(ctx context.Context, msg string, err error, attrs ...slog.Attr) {
	Get().ErrorAttrs(ctx, msg, err, attrs...)
}
