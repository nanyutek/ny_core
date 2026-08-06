# 📖 `nylog` 生产级 Go 模块库落地完全指导与配置手册

基于 **Go 1.21+ 标准库 `log/slog`** 打造的生产级、高吞吐、强类型安全的结构化日志组件库。汲取了 Uber `zap` 与 Go 社区开源日志的最佳实践，专注于**零丢日志、高可运维性、强类型防错与敏感数据安全防护**。

模块引用路径：`github.com/nanyutek/ny_core/nylog`

---

## 目录

1. [🚀 快速集成指南 (Step-by-Step)](#-快速集成指南-step-by-step)
2. [⚙️ 核心配置项 (`Config`) 逐项详解与生产指南](#%EF%B8%8F-核心配置项-config-逐项详解与生产指南)
   * [基础配置 (`Level`, `Format`, `EnableConsole`)](#1-基础输出配置)
   * [调优与堆栈配置 (`EnableAddSource`, `PrintErrorStack`)](#2-性能调优与堆栈配置)
   * [主日志文件切割归档 (`File.Filename`, `File.MaxSizeMB`, `File.MaxBackups`, `File.MaxAgeDays`, `File.Compress`)](#3-主日志文件切割归档配置-fileconfig)
   * [高级存储隔离 (`ErrorFilename`, `AttachLogDir`)](#4-高级日志隔离配置)
3. [💡 核心 API 与方法使用指导手册](#-核心-api-与方法使用指导手册)
   * [3.1 强类型日志打印（彻底规避 `!BADKEY`）](#31-强类型日志打印彻底规避-badkey)
   * [3.2 敏感数据脱敏指南（专用助手与 `LogValuer` 自动隐蔽）](#32-敏感数据脱敏指南专用助手与-logvaluer-自动隐蔽)
   * [3.3 分布式 TraceID 与 Context 动态属性关联](#33-分布式-traceid-与-context-动态属性关联)
   * [3.4 模块日志分流 (`Attach` 与 `Detach`)](#34-模块日志分流-attach-与-detach)
   * [3.5 崩溃拦截防护 (`CatchPanic`)](#35-崩溃拦截防护-catchpanic)
   * [3.6 优雅关机与缓冲区强制刷盘 (`Sync`)](#36-优雅关机与缓冲区强制刷盘-sync)
   * [3.7 标准库桥接 (`ToStdLogger`)](#37-标准库桥接-tostdlogger)
4. [🛠️ Option 扩展机制（第三方 Handler 与写入异常告警）](#%EF%B8%8F-option-扩展机制第三方-handler-与写入异常告警)
5. [🛡️ 代码规范约束 (`golangci-lint` 集成)](#-代码规范约束-golangci-lint-集成)
6. [📦 独立发布与 Go Module 拆包指引](#-独立发布与-go-module-拆包指引)

---

## 🚀 快速集成指南 (Step-by-Step)

### 第一步：在项目中引入模块

在需要使用日志的 Go 项目 `go.mod` 中确认已包含依赖：

```bash
go get github.com/nanyutek/ny_core/nylog
```

### 第二步：应用启动时完成初始化

在服务入口（`main.go` 或 `bootstrap.go`）中进行初始化，并利用 `defer nylog.Sync()` 挂载优雅关机刷盘：

```go
package main

import (
	"context"

	"github.com/nanyutek/ny_core/nylog"
)

func main() {
	// 1. 使用默认生产推荐配置完成全局 Logger 初始化
	_ = nylog.InitLogger(nylog.DefaultConfig())

	// 2. 注册程序退出前的强制刷盘 (防止缓冲区日志丢失)
	defer nylog.Sync()

	ctx := context.Background()

	// 3. 开始打印结构化日志
	nylog.InfoAttrs(ctx, "微服务启动成功",
		nylog.String("service", "order-center"),
		nylog.Int("port", 8080),
	)
}
```

---

## ⚙️ 核心配置项 (`Config`) 逐项详解与生产指南

日志配置体 `Config` 包含 12 个关键控制字段，每个字段的作用、默认值及生产环境建议如下：

```go
type Config struct {
	Level           string     `json:"level" yaml:"level"`
	Format          FileFormat `json:"format" yaml:"format"`
	EnableConsole   bool       `json:"enable_console" yaml:"enable_console"`
	EnableAddSource *bool      `json:"enable_add_source" yaml:"enable_add_source"`
	PrintErrorStack bool       `json:"print_error_stack" yaml:"print_error_stack"`
	File            FileConfig `json:"file" yaml:"file"`
	ErrorFilename   string     `json:"error_filename" yaml:"error_filename"`
	AttachLogDir    string     `json:"attach_log_dir" yaml:"attach_log_dir"`
}
```

### 1. 基础输出配置

#### 🔹 `Level` (日志过滤最低级别)
* **作用**：控制系统最低输出哪一个级别的日志。低级别日志会被自动忽略，不参与 CPU 拼接与磁盘写入。
* **可选值**：`"debug"`、`"info"`、`"warn"`、`"error"`（不区分大小写）。
* **默认值**：`"info"`。
* **生产指导**：
  * **线上生产环境**：强烈建议设为 `"info"` 或 `"warn"`。设为 debug 会导致日志量暴增，刷满磁盘。
  * **预发/测试环境**：设为 `"debug"` 方便排查链路问题。
  * **动态调优**：运行期若需要临时排查，可直接调用 `nylog.SetLevel("debug")` 进行在线级别切换。

#### 🔹 `Format` (日志输出物理格式)
* **作用**：决定日志输出时的序列化编码方式。
* **可选值**：`nylog.FileFormatJSON`（`"json"`）或 `nylog.FileFormatText`（`"text"`）。
* **默认值**：`nylog.FileFormatJSON`。
* **生产指导**：
  * **生产环境**：**必须设为 `"json"`**。JSON 格式能够被 ELK、Loki、Vector、Filebeat 等日志检索系统无缝解析和字段检索。
  * **本地开发环境**：可设为 `"text"`，在控制台更直观可读。

#### 🔹 `EnableConsole` (是否同时打印到控制台 `os.Stdout`)
* **作用**：布尔值开关。开启后，日志在写入文件的同时，也会打印一份到标准的 `os.Stdout` 控制台。
* **默认值**：`true`。
* **生产指导**：
  * **K8s / 容器化部署**：建议保持 `true`。K8s 依赖容器控制台标准输出捕获平台日志。
  * **传统物理机/虚拟机部署**：如果已配置写磁盘文件 `File.Filename`，且物理机上了日志收集 Agent，可关闭设为 `false` 以减轻终端 I/O 开销。

---

### 2. 性能调优与堆栈配置

#### 🔹 `EnableAddSource` (是否记录源码文件行号)
* **作用**：指针类型布尔值。开启后，日志中会新增 `"caller"` 字段，记录调用日志的代码相对位置（如 `order/service.go:42`）。
* **默认值**：`&true`（默认开启）。
* **生产指导**：
  * **常规生产**：建议保留开启 (`true`)。精确的代码行号对于定位故障点至关重要。
  * **极致高并发/高性能**：获取代码位置需要调用 CPU 栈帧识别，在极端吞吐（>10万 QPS）场景下，可显式设置为 `false` 以节省 CPU 耗时。

#### 🔹 `PrintErrorStack` (Error 级别是否自动抓取堆栈)
* **作用**：布尔值。控制调用 `ErrorAttrs` 打印错误时，是否在 JSON 中自动挂载 `"stack"` 堆栈字段。
* **默认值**：`true`。
* **生产指导**：
  * **推荐做法**：`nylog` 的实现做了性能优化——若错误对象本身包含原始堆栈（如 `pkg/errors` 或 `xerrors` 包装的错误），会直接提炼发源地堆栈；仅当 error 未自带堆栈时才会调用 `debug.Stack()` 捕获。
  * 若生产环境属于高频预期业务报错（如用户密码输入错误），建议用 `WarnAttrs` 打印，不要滥用 `ErrorAttrs`，避免高频生成 Stack 增加内存压力。

---

### 3. 主日志文件切割归档配置 (`FileConfig`)

针对主日志文件 `File` 的自动化体积控制与旧日志清理：

```go
type FileConfig struct {
	Filename   string `json:"filename" yaml:"filename"`
	MaxSizeMB  int    `json:"max_size_mb" yaml:"max_size_mb"`
	MaxBackups int    `json:"max_backups" yaml:"max_backups"`
	MaxAgeDays int    `json:"max_age_days" yaml:"max_age_days"`
	Compress   bool   `json:"compress" yaml:"compress"`
}
```

#### 🔹 `File.Filename` (主日志物理存储路径)
* **作用**：主日志文件绝对路径或相对路径（如 `"logs/app.log"`）。若传空字符串 `""`，则不开启文件输出。
* **默认值**：`"logs/app.log"`。
* **生产指导**：生产环境中应确保应用对该目录拥有读写权限。支持相对路径或如 `/var/log/my_app/app.log` 的绝对路径。

#### 🔹 `File.MaxSizeMB` (单个日志文件最大体积限制)
* **作用**：整数（单位：MB）。当当前日志文件大小达到该阈值时，`lumberjack` 会自动关闭当前文件并生成归档。
* **默认值**：`100` (即 100MB)。边界值处理：若设置 `<= 0`，框架会自动兜底修正为 `100MB`。
* **生产指导**：建议设置为 `100MB` ~ `500MB`。文件太大会导致文本编辑器或分析工具难以打开，太小会导致归档文件数量过于密集。

#### 🔹 `File.MaxBackups` (归档日志最大保留份数)
* **作用**：整数。控制保留的最大旧日志文件个数。
* **默认值**：`10`。若设为 `0`，则表示不限制保留个数。
* **生产指导**：根据磁盘容量设定，一般设置为 `10` ~ `30` 份。

#### 🔹 `File.MaxAgeDays` (归档日志最大保留天数)
* **作用**：整数（单位：天）。根据创建时间删除超过指定天数的旧日志文件。
* **默认值**：`30` (即保留 30 天)。若设为 `0`，则表示不按天数自动删除。
* **生产指导**：按照合规安全要求（如等保要求日志留存不少于 6 个月），若物理机磁盘允许，生产建议设为 `30` 到 `90` 天；若已有统一中央日志收集系统收集，本地可设为 `7` 到 `15` 天以释放磁盘空间。

#### 🔹 `File.Compress` (归档日志是否使用 Gzip 压缩)
* **作用**：布尔值。历史旧日志文件切分后，是否自动压缩为 `.log.gz`。
* **默认值**：`true`。
* **生产指导**：**强烈建议保持 `true`**。Gzip 对文本日志文件的压缩率通常可达到 **85% 以上**，能节省巨大的物理磁盘空间。

---

### 4. 高级日志隔离配置

#### 🔹 `ErrorFilename` (独立 Error 级别日志归档路径)
* **作用**：字符串路径（如 `"logs/error.log"`）。开启后，所有 `ERROR` 级别的日志除了打入主日志外，会**额外独立汇总一份到此文件**中。
* **默认值**：`""` (不开启)。
* **生产指导**：运维或监控非常喜爱的配置。设为 `"logs/error.log"` 后，排查人员只需查看该文件即可了解线上所有报错，免去在全量日志中 grep 筛选的麻烦。

#### 🔹 `AttachLogDir` (附加与隔离模块日志的存储目录)
* **作用**：目录路径字符串。在使用 `Attach(module)` 或 `Detach(module)` 派生子 Logger 时，模块日志文件的统一存放目录。
* **默认值**：`"logs"`。
* **生产指导**：如 `Attach("payment")` 会生成 `"logs/payment.log"`，`Detach("audit")` 会生成 `"logs/detach_audit.log"`。

---

## 💡 核心 API 与方法使用指导手册

### 3.1 强类型日志打印（彻底规避 `!BADKEY`）

为了彻底杜绝原生 `slog` 动态变参 `...any` 导致的键值对缺少产生 `!BADKEY` 坏键问题，`nylog` 提供了**编译期强类型检出的 `Attrs` API**：

```go
func (l *Logger) DebugAttrs(ctx context.Context, msg string, attrs ...slog.Attr)
func (l *Logger) InfoAttrs(ctx context.Context, msg string, attrs ...slog.Attr)
func (l *Logger) WarnAttrs(ctx context.Context, msg string, attrs ...slog.Attr)
func (l *Logger) ErrorAttrs(ctx context.Context, msg string, err error, attrs ...slog.Attr)
```

#### 范例：使用强类型 Attrs 助手构建日志

```go
// 引入属性构建助手: nylog.String, nylog.Int, nylog.Bool, nylog.Err
nylog.InfoAttrs(ctx, "用户发起提现申请",
	nylog.String("user_id", "U9527"),
	nylog.Int64("amount_cents", 50000),
	nylog.Bool("is_vip", true),
)

// 发生错误时的标准打印：
err := repository.ErrRecordNotFound
nylog.ErrorAttrs(ctx, "查询账户信息失败", err,
	nylog.String("user_id", "U9527"),
)
```

---

### 3.2 敏感数据脱敏指南（专用助手与 `LogValuer` 自动隐蔽）

针对生产合规，`nylog` 提供了**手动专有助手脱敏**与**领域模型自动脱敏**双重保障：

#### 方式一：专有助手函数 (字符串脱敏)

针对常见敏感属性，提供开箱即用的格式化脱敏转换：

```go
nylog.InfoAttrs(ctx, "用户实名认证提交",
	nylog.MaskMobile("phone", "13800138000"),       // 输出: "phone":"138****8000"
	nylog.MaskEmail("email", "zhangsan@qq.com"),   // 输出: "email":"z***n@qq.com"
	nylog.MaskIDCard("id_card", "110101199003071234"),// 输出: "id_card":"110101******1234"
	nylog.MaskBankCard("card_no", "6222021204000001234"),// 输出: "card_no":"622202******1234"
)
```

#### 方式二：领域模型自动隐蔽 (`slog.LogValuer` 接口)

当需要将一个 Struct 整体打印到日志中时，只需让该 Struct 实现 `slog.LogValuer` 接口。**就算开发人员直接打印对象，里面的敏感密码也绝对不会在日志中泄露**：

```go
// 1. 在 Domain 模型中定义 LogValue 方法
type SensitiveUser struct {
	ID       string
	Name     string
	Password string // 敏感字段
}

func (u SensitiveUser) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("id", u.ID),
		slog.String("name", u.Name),
		slog.String("password", "******"), // 自动转换为遮罩
	)
}

// 2. 业务使用中直接把 User 对象打入日志：
user := SensitiveUser{ID: "U001", Name: "张三", Password: "MySecretPassword123"}
nylog.InfoAttrs(ctx, "用户登录信息", slog.Any("user_info", user))

// 物理输出 JSON 结果:
// {"msg":"用户登录信息","user_info":{"id":"U001","name":"张三","password":"******"}}
```

---

### 3.3 分布式 TraceID 与 Context 动态属性关联

高并发微服务调用链中，通常需要在日志中自动带出 `trace_id` 或中间件提取的公共字段。

#### 场景 1：自动提取默认 TraceID
`nylog` 内置了默认提取器。如果 `context.Context` 中存在 `"trace_id"` 或 `"request_id"` 的字符串 Key，会自动被提炼并加进日志中：

```go
ctx := context.WithValue(context.Background(), "trace_id", "TRACE-889900-X")
nylog.InfoAttrs(ctx, "处理网关请求")
// 输出 JSON 中会自动附带: "trace_id":"TRACE-889900-X"
```

#### 场景 2：绑定自定义 Context Key
如果你的微服务框架使用自定义类型的 Context Key：

```go
type customKey string
const MyTraceKey customKey = "X-My-Trace-Id"

func main() {
	// 在启动时注册自定义的 TraceKey
	_ = nylog.InitLogger(
		nylog.DefaultConfig(),
		nylog.WithTraceKey(MyTraceKey, "custom_trace_id"),
	)

	ctx := context.WithValue(context.Background(), MyTraceKey, "TRACE-ABC-123")
	nylog.InfoAttrs(ctx, "执行订单结算")
	// 输出 JSON 中自动追加: "custom_trace_id":"TRACE-ABC-123"
}
```

#### 场景 3：HTTP 中间件中给 Context 动态绑入属性
使用 `nylog.WithContextAttr(parentCtx, attrs...)` 可以在请求入口中将任意属性（如 `tenant_id`）一次性绑进 Context，后续该请求链路调用的所有日志都会自动带上这些属性：

```go
func Middleware(c *gin.Context) {
	// 往 Context 中绑定租户与请求入口属性
	ctx := nylog.WithContextAttr(c.Request.Context(),
		nylog.String("tenant_id", c.GetHeader("X-Tenant-ID")),
		nylog.String("path", c.Request.URL.Path),
	)
	c.Request = c.Request.WithContext(ctx)
	c.Next()
}
```

---

### 3.4 模块日志分流 (`Attach` 与 `Detach`)

为了满足大型复杂系统中子模块日志独立归档的诉求，提供两种子通道派生模式：

```
                              ┌──> 主日志 (logs/app.log)
               ┌── Attach ───┤
               │              └──> 模块日志 (logs/payment.log)
父 Logger ────┤
               │              ┌──> 屏蔽主日志 (不写 app.log)
               └── Detach ───┤
                              └──> 专属日志 (logs/detach_audit.log)
```

```go
// 1. Attach 附加模式：既打入主日志 app.log，也同步写进 logs/payment.log
payLogger := nylog.Attach("payment")
payLogger.InfoAttrs(ctx, "支付异步回调通知成功")

// 2. Detach 隔离模式：不写主日志 app.log，仅写进 logs/detach_cron.log + 控制台
cronLogger := nylog.Detach("cron")
cronLogger.InfoAttrs(ctx, "定时任务开始清理垃圾数据")
```

---

### 3.5 崩溃拦截防护 (`CatchPanic`)

避免因为 Goroutine 内突发的未捕获 panic 导致整个微服务进程宕机崩溃。

```go
func SafeGoroutine(ctx context.Context) {
	// 挂载 CatchPanic 拦截器，自动 recover 并在日志中录入带 Stack 的错误信息
	defer nylog.CatchPanic(ctx)

	// 模拟引发空指针异常
	var ptr *int
	*ptr = 100
}
```

---

### 3.6 优雅关机与缓冲区强制刷盘 (`Sync`)

为了保证微服务在收到 K8s 的 `SIGTERM` 关机信号时，留在内存写缓冲区的日志不丢失，必须在程序退出前调用 `Sync()`：

```go
func main() {
	_ = nylog.InitLogger(nylog.DefaultConfig())
	
	// 挂载刷盘钩子，优雅退出时同步刷新写磁盘指针
	defer nylog.Sync()

	// 业务逻辑...
}
```

---

### 3.7 标准库桥接 (`ToStdLogger`)

当第三方的开源框架（如 `http.Server.ErrorLog`、GORM 初始配置）只支持 Go 标准库的 `*log.Logger` 时，可通过此 API 进行零成本桥接：

```go
httpServer := &http.Server{
	Addr:     ":8080",
	// 将 nylog 的 Error 管道桥接给标准的 http.Server 告警使用
	ErrorLog: nylog.ToStdLogger(slog.LevelError),
}
```

---

## 🛠️ Option 扩展机制（第三方 Handler 与写入异常告警）

在 `InitLogger(conf, options...)` 初始化时，支持传入可选的 Option 拓展：

### 1. 注入第三方扩展 Handler (`WithExtraHandler`)
允许在不修改 `nylog` 源码的前提下，插入自定义的 Handler（如将 Error 同步投递给 Sentry/Kafka/ES，或者在本地使用彩色的 `tint` Handler）：

```go
// 示例：注入一个彩色的控制台 Handler 用于本地开发
colorHandler := tint.NewHandler(os.Stdout, &tint.Options{Level: slog.LevelDebug})

_ = nylog.InitLogger(
	nylog.DefaultConfig(),
	nylog.WithExtraHandler(colorHandler), // 插件式扩展
)
```

### 2. 注入全局属性加工转换钩子 (`WithReplaceAttr`)
允许在全站日志输出前修改特定的 Attribute（如全局修改 Key 命名风格、隐蔽全局固定关键词等）：

```go
_ = nylog.InitLogger(
	nylog.DefaultConfig(),
	nylog.WithReplaceAttr(func(groups []string, a slog.Attr) slog.Attr {
		if a.Key == "server_ip" {
			return slog.String("server_ip", "10.0.0.1")
		}
		return a
	}),
)
```

### 3. 注入日志落盘失败报警回调 (`WithWriteErrorHandler`)
当底层磁盘被刷满、发生 Write Permission Error 等严重故障导致写入失败时，触发用户自定义回调（防止日志丢失而应用无感知）：

```go
_ = nylog.InitLogger(
	nylog.DefaultConfig(),
	nylog.WithWriteErrorHandler(func(err error, r slog.Record) {
		// 当物理磁盘满导致写失败时，触发应急监控或推送
		metrics.IncrCounter("log_write_errors", 1)
	}),
)
```

---

## 🛡️ 代码规范约束 (`golangci-lint` 集成)

为了防止团队成员混用弱类型的 `slog.Info()` 导致 `!BADKEY` 坏键问题，推荐在项目根目录 `.golangci.yml` 中加上 `sloglint` 的强制静态检查：

```yaml
linters:
  enable:
    - sloglint

linters-settings:
  sloglint:
    attr-only: true          # 强制禁止使用 k-v 交叉传参，必须使用 LogAttrs/Attr API
    no-global: "all"         # 约束代码中尽量通过注入或规范方法使用
    context: "all"           # 强制所有日志打印必须显式传入 Context 参数
    static-msg: true         # 强制日志 message 必须为静态字符串字面量
    key-naming-case: snake   # 日志属性 Key 统一小写蛇形 (order_id)
```

---

## 📦 独立发布与 Go Module 拆包指引

如果未来需要将 `nylog` 进一步抽离为全公司通用的独立开源组件仓库（例如 `github.com/nanyutek/nylog`）：

1. **进入包所在目录并创建独立的 `go.mod`**：
   ```bash
   cd path/to/nylog
   go mod init github.com/nanyutek/nylog
   go mod tidy
   ```
2. **打 Tag 并发布**：
   ```bash
   git tag v1.0.0
   git push origin v1.0.0
   ```
3. **在其他外部业务项目中直接通过标准 Module 引用**：
   ```bash
   go get github.com/nanyutek/nylog@v1.0.0
   ```
