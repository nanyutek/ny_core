package nylog

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
)

type Logger struct {
	*slog.Logger
	conf     Config
	levelVar *slog.LevelVar
}

var (
	defaultGlobal atomic.Pointer[Logger]
	globalMutex   sync.Mutex
)

type Options struct {
	extraHandlers     []slog.Handler
	replaceAttrs      []func(groups []string, a slog.Attr) slog.Attr
	contextExtractors []ContextExtractor
}

type Option func(*Options)

// WithExtraHandler 允许用户注入自定义的 slog.Handler (如将 Error 日志发给 Kafka/ES/Sentry 或彩色控制台)
func WithExtraHandler(handlers ...slog.Handler) Option {
	return func(o *Options) {
		o.extraHandlers = append(o.extraHandlers, handlers...)
	}
}

// WithReplaceAttr 允许用户自定义属性转换钩子 (如特定键名替换或补充敏感词过滤)
func WithReplaceAttr(fn func(groups []string, a slog.Attr) slog.Attr) Option {
	return func(o *Options) {
		o.replaceAttrs = append(o.replaceAttrs, fn)
	}
}

// WithContextExtractor 允许用户自定义从 Context 提取 TraceID/SpanID/UserID 等属性的转换提炼器
func WithContextExtractor(fn ContextExtractor) Option {
	return func(o *Options) {
		o.contextExtractors = append(o.contextExtractors, fn)
	}
}

// WithTraceKey 允许用户指定自定义的 Context Key 类型和输出的日志字段 Key（attrKey 留空默认为 "trace_id"）
func WithTraceKey(ctxKey any, attrKey string) Option {
	if attrKey == "" {
		attrKey = "trace_id"
	}
	return func(o *Options) {
		o.contextExtractors = append(o.contextExtractors, func(ctx context.Context) []slog.Attr {
			if ctx == nil {
				return nil
			}
			if val, ok := ctx.Value(ctxKey).(string); ok && val != "" {
				return []slog.Attr{slog.String(attrKey, val)}
			}
			return nil
		})
	}
}

// InitLogger 全局日志初始化 (支持 Functional Options 扩展)
func InitLogger(conf Config, options ...Option) *Logger {
	globalMutex.Lock()
	defer globalMutex.Unlock()

	optsImpl := &Options{}
	for _, opt := range options {
		opt(optsImpl)
	}

	levelVar := new(slog.LevelVar)
	levelVar.Set(parseLevel(conf.Level))

	opts := &slog.HandlerOptions{
		Level:     levelVar,
		AddSource: true,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// 执行用户注入的 ReplaceAttr 钩子
			for _, fn := range optsImpl.replaceAttrs {
				a = fn(groups, a)
			}
			if a.Key == slog.TimeKey {
				return slog.String(slog.TimeKey, a.Value.Time().Format("2006-01-02 15:04:05.000"))
			}
			if a.Key == slog.SourceKey {
				if source, ok := a.Value.Any().(*slog.Source); ok {
					return slog.String("caller", fmt.Sprintf("%s:%d", filepath.Base(source.File), source.Line))
				}
			}
			return a
		},
	}

	var handlers []slog.Handler

	if conf.EnableConsole {
		if conf.Format == FileFormatJSON {
			handlers = append(handlers, slog.NewJSONHandler(os.Stdout, opts))
		} else {
			handlers = append(handlers, slog.NewTextHandler(os.Stdout, opts))
		}
	}

	if conf.File.Filename != "" {
		fileWriter := getLumberjackWriter(conf.File.Filename, conf.File)
		handlers = append(handlers, slog.NewJSONHandler(fileWriter, opts))
	}

	// 注入用户自定义的扩展 Handler (如第三方 ES / Kafka / Sentry / Tint 处理器)
	if len(optsImpl.extraHandlers) > 0 {
		handlers = append(handlers, optsImpl.extraHandlers...)
	}

	tee := NewTeeHandler(handlers...).WithExtractors(optsImpl.contextExtractors...)
	l := slog.New(tee)

	inst := &Logger{
		Logger:   l,
		conf:     conf,
		levelVar: levelVar,
	}

	defaultGlobal.Store(inst)
	slog.SetDefault(l)
	return inst
}

func Get() *Logger {
	if l := defaultGlobal.Load(); l != nil {
		return l
	}
	return InitLogger(DefaultConfig())
}

// Sync 强行刷新日志磁盘指针 (主要用于微服务优雅关机时呼叫)
func (l *Logger) Sync() error {
	return SyncAll()
}

// ToStdLogger 桥接导出标准库 *log.Logger (方便传递给 http.Server.ErrorLog 等老 API)
func (l *Logger) ToStdLogger(level slog.Level) *log.Logger {
	return slog.NewLogLogger(l.Handler(), level)
}

// --- 推荐使用的 LogAttrs 类型安全 API (编译期检查) ---

func (l *Logger) InfoAttrs(ctx context.Context, msg string, attrs ...slog.Attr) {
	l.LogAttrs(ctx, slog.LevelInfo, msg, attrs...)
}

func (l *Logger) DebugAttrs(ctx context.Context, msg string, attrs ...slog.Attr) {
	l.LogAttrs(ctx, slog.LevelDebug, msg, attrs...)
}

func (l *Logger) WarnAttrs(ctx context.Context, msg string, attrs ...slog.Attr) {
	l.LogAttrs(ctx, slog.LevelWarn, msg, attrs...)
}

// Attach 附加日志：主日志写入的同时，另写一份到 attach_log_dir/module.log
func (l *Logger) Attach(moduleName string) *Logger {
	if moduleName == "" {
		return l
	}
	targetPath := filepath.Join(l.conf.AttachLogDir, fmt.Sprintf("%s.log", moduleName))
	fileWriter := getLumberjackWriter(targetPath, l.conf.File)

	subHandler := slog.NewJSONHandler(fileWriter, &slog.HandlerOptions{
		Level:     l.levelVar,
		AddSource: true,
	})

	return &Logger{
		Logger:   slog.New(NewTeeHandler(l.Handler(), subHandler)).With("module", moduleName),
		conf:     l.conf,
		levelVar: l.levelVar,
	}
}

// Detach 隔离日志：不写入主日志，仅写入 attach_log_dir/detach_module.log 和控制台
func (l *Logger) Detach(moduleName string) *Logger {
	if moduleName == "" {
		return l
	}
	targetPath := filepath.Join(l.conf.AttachLogDir, fmt.Sprintf("detach_%s.log", moduleName))
	fileWriter := getLumberjackWriter(targetPath, l.conf.File)

	opts := &slog.HandlerOptions{Level: l.levelVar, AddSource: true}
	var handlers []slog.Handler

	if l.conf.EnableConsole {
		handlers = append(handlers, slog.NewJSONHandler(os.Stdout, opts))
	}
	handlers = append(handlers, slog.NewJSONHandler(fileWriter, opts))

	return &Logger{
		Logger:   slog.New(NewTeeHandler(handlers...)).With("module", moduleName),
		conf:     l.conf,
		levelVar: l.levelVar,
	}
}

// --- 包级别顶层快捷 API (直接代理全局单例，进一步提升开发体验) ---
func Sync() error {
	return Get().Sync()
}

func InfoAttrs(ctx context.Context, msg string, attrs ...slog.Attr) {
	Get().InfoAttrs(ctx, msg, attrs...)
}

func DebugAttrs(ctx context.Context, msg string, attrs ...slog.Attr) {
	Get().DebugAttrs(ctx, msg, attrs...)
}

func WarnAttrs(ctx context.Context, msg string, attrs ...slog.Attr) {
	Get().WarnAttrs(ctx, msg, attrs...)
}

func Attach(moduleName string) *Logger {
	return Get().Attach(moduleName)
}

func Detach(moduleName string) *Logger {
	return Get().Detach(moduleName)
}

func ToStdLogger(level slog.Level) *log.Logger {
	return Get().ToStdLogger(level)
}
