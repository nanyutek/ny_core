# 📦 Enterprise Go Logging Component (`nylog`)

基于 **Go 1.21+ 标准库 `log/slog`** 打造的生产级、轻量化、高扩展性结构化日志框架。汲取了 Uber `zap` 与开源日志社区最佳实践，兼具极致性能与强类型安全。

模块路径：`github.com/nanyutek/ny_core/nylog`

---

## 🌟 核心特性 (Key Features)

* **零依赖原生标准**：基于 Go 官方 `log/slog` 设计，性能与内存优化极其优秀。
* **物理防错（规避 `!BADKEY`）**：全面采用强类型 `LogAttrs` / `Attr` 参数绑定，在**编译期**彻底消灭变参 `...any` 导致的键值对颠倒与缺失问题。
* **自动敏感数据脱敏**：无缝支持 Go 官方 `slog.LogValuer` 接口，领域模型实现该接口后自动脱敏（如密码、身份证、银行卡），防止明文泄露。
* **句柄池化保护**：内置 `sync.Map` 管理 `lumberjack` 文件切割器，高并发与多模块派生下保证**零文件句柄泄漏与写竞争**。
* **子通道派生 (Attach & Detach)**：
  * `Attach`: 主日志写入的同时，同步抄送一份到特定模块日志文件。
  * `Detach`: 隔离日志，仅输出到独立模块文件，不干扰主日志文件。
* **高度可扩展 (Functional Options)**：支持通过 Option 模式自由注入外部第三方 `slog.Handler`（如发往 Kafka/ES/Sentry，或终端彩色 `tint` 输出）。
* **动态 TraceID 与 Context 提取**：内置默认回退提取逻辑（查找 `trace_id` / `request_id`），同时支持自定义 Context Key 或全局 Extract 函数。
* **高并发无锁获取 (Atomic Pointer)**：全局单例基于 `atomic.Pointer` 实现无锁原子加载，消除高并发场景下的读锁开销。
* **优雅关机刷盘 (`Sync`)**：暴露 `Sync()` 接口，在微服务退出时强制刷盘缓冲日志，避免关键关机日志丢失。
* **标准库无缝桥接**：提供 `ToStdLogger()` 接口导出标准库 `*log.Logger` 句柄，方便给 `http.Server.ErrorLog` 或 GORM 等第三方框架复用。

---

## 🚀 快速开始 (Quick Start)

### 1. 配置文件初始化与使用

```go
package main

import (
	"context"

	"github.com/nanyutek/ny_core/nylog"
)

func main() {
	// 1. 初始化全局日志配置
	log := nylog.InitLogger(nylog.DefaultConfig())
	defer nylog.Sync() // 微服务退出时自动刷盘文件句柄

	ctx := context.Background()

	// 2. 包级静态快捷调用 (推荐)
	nylog.InfoAttrs(ctx, "服务启动成功",
		nylog.String("app_name", "my_service"),
		nylog.Int("port", 8080),
	)

	// 3. 实例方法调用
	log.InfoAttrs(ctx, "实例处理事件", nylog.String("event", "init"))
}
```

---

## ⚙️ 配置说明 (Configuration)

配置结构体定义在 `config.go` 中，支持通过 YAML / INI 文件动态反序列化：

```go
type Config struct {
	Level           string     `json:"level" yaml:"level"`                        // debug, info, warn, error
	Format          FileFormat `json:"format" yaml:"format"`                      // json 或 text
	EnableConsole   bool       `json:"enable_console" yaml:"enable_console"`      // 是否同时输出到控制台
	PrintErrorStack bool       `json:"print_error_stack" yaml:"print_error_stack"`// Error 级别是否打印堆栈 (Stack)
	File            FileConfig `json:"file" yaml:"file"`                          // 主日志文件切割配置
	AttachLogDir    string     `json:"attach_log_dir" yaml:"attach_log_dir"`      // 模块日志存储目录
}

type FileConfig struct {
	Filename   string `json:"filename" yaml:"filename"`       // 文件路径，如 logs/app.log
	MaxSizeMB  int    `json:"max_size_mb" yaml:"max_size_mb"` // 单文件最大大小 (MB)
	MaxBackups int    `json:"max_backups" yaml:"max_backups"` // 最大留存备份文件数
	MaxAgeDays int    `json:"max_age_days" yaml:"max_age_days"` // 备份留存最大天数
	Compress   bool   `json:"compress" yaml:"compress"`       // 备份是否 gzip 压缩
}
```

---

## 💡 高级用法范例 (Advanced Usage)

### 1. 结构化属性与通用脱敏 (Attrs & Masking)

```go
errSample := errors.New("connection reset")

nylog.InfoAttrs(ctx, "数据推送处理",
    nylog.String("service", "order_service"),
    nylog.Int("retry_count", 3),
    nylog.Err(errSample),                  // 优雅包装 error，自动忽略 nil
    nylog.Mask("mobile", "13800138000"),  // 自动脱敏为 138****8000
)
```

### 2. 领域对象自动脱敏 (`slog.LogValuer`)

实现 `slog.LogValuer` 接口的对象在打印时会自动脱敏：

```go
type User struct {
	ID       string
	Password string
}

func (u User) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("id", u.ID),
		slog.String("password", "******"), // 强制遮罩
	)
}

// 调用时直接传入结构体：
nylog.InfoAttrs(ctx, "用户身份验证", slog.Any("user_info", User{ID: "U001", Password: "raw_password"}))
```

### 3. 自定义 TraceID Context 提取

```go
type myCtxKey string
const TraceKey myCtxKey = "X-Trace-Id"

func main() {
	log := nylog.InitLogger(
		nylog.DefaultConfig(),
		nylog.WithTraceKey(TraceKey, "trace_id"), // 指定 Context Key 及输出的日志属性 Key
	)

	ctx := context.WithValue(context.Background(), TraceKey, "TRACE-998123")
	log.InfoAttrs(ctx, "收到微服务请求")
	// 自动包含: "trace_id": "TRACE-998123"
}
```

### 4. 模块日志分流 (Attach & Detach)

```go
// Attach: 既打入 logs/app.log，也同步打入 logs/payment.log
payLog := nylog.Attach("payment")
payLog.InfoAttrs(ctx, "收到第三方支付异步回调")

// Detach: 不写入主日志 app.log，独立打入 logs/detach_cron.log
cronLog := nylog.Detach("cron")
cronLog.InfoAttrs(ctx, "定时任务开启清理")
```

### 5. 崩溃拦截组件 (Panic Catcher)

在 Gin 中间件或后台 Go 协程中：

```go
func WorkerThread(ctx context.Context) {
	defer nylog.CatchPanic(ctx) // 自动 recover 并打印结构化 Stack

	panic("unexpected nil pointer")
}
```
