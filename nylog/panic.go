package nylog

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
)

type stackTracer interface {
	StackTrace() string
}

// ErrorAttrs 包含堆栈信息判断的强类型 Error 日志
func (l *Logger) ErrorAttrs(ctx context.Context, msg string, err error, attrs ...slog.Attr) {
	if err != nil {
		attrs = append(attrs, Err(err))

		// 1. 优先从支持 StackTrace() 的 error 中提取发源地原始堆栈 (如 pkg/errors)
		if tracer, ok := err.(stackTracer); ok {
			attrs = append(attrs, slog.String("stack", tracer.StackTrace()))
		} else if l.conf.PrintErrorStack {
			// 2. 降级：仅当配置 PrintErrorStack 为 true 且 error 未自带堆栈时，采集当前堆栈
			attrs = append(attrs, slog.String("stack", string(debug.Stack())))
		}
	} else if l.conf.PrintErrorStack {
		attrs = append(attrs, slog.String("stack", string(debug.Stack())))
	}

	l.LogAttrs(ctx, slog.LevelError, msg, attrs...)
}

// CatchPanic 崩溃拦截器组件
func CatchPanic(ctx context.Context) {
	if r := recover(); r != nil {
		var err error
		if e, ok := r.(error); ok {
			err = e
		} else {
			err = fmt.Errorf("%v", r)
		}
		Get().ErrorAttrs(ctx, "[SYSTEM PANIC INTERCEPTED]", err,
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
