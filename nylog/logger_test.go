package nylog

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func getTestTmpDir(t *testing.T) string {
	_ = os.MkdirAll(".test_tmp", 0755)
	dir, err := os.MkdirTemp(".test_tmp", "test-*")
	if err != nil {
		t.Fatalf("Failed to create in-workspace temp dir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
	})
	return dir
}

// TestInitAndGetLogger 测试 Logger 单例初始化与 Get 获取句柄功能
func TestInitAndGetLogger(t *testing.T) {
	tmpDir := getTestTmpDir(t)
	cfg := Config{
		Level:           "debug",
		Format:          "json",
		EnableConsole:   false,
		PrintErrorStack: false,
		AttachLogDir:    tmpDir,
		File: FileConfig{
			Filename:  filepath.Join(tmpDir, "test.log"),
			MaxSizeMB: 10,
		},
	}

	logger := InitLogger(cfg)
	if logger == nil {
		t.Fatal("InitLogger 初始化失败返回 nil")
	}

	if Get() != logger {
		t.Fatal("Get() 未能准确获取已初始化的全局单例 Logger")
	}
}

// TestAttrsAndMasking 测试强类型 Attr 构建器（String, Int, Err, Mask 等）及脱敏助手
func TestAttrsAndMasking(t *testing.T) {
	ctx := context.Background()
	tmpDir := getTestTmpDir(t)
	logFile := filepath.Join(tmpDir, "test_attrs.log")

	cfg := Config{
		Level:         "debug",
		Format:        "json",
		EnableConsole: false,
		File:          FileConfig{Filename: logFile},
	}
	logger := InitLogger(cfg)

	errSample := errors.New("sample error")
	logger.InfoAttrs(ctx, "测试属性输出",
		String("str_key", "hello"),
		Int("int_key", 42),
		Bool("bool_key", true),
		Err(errSample),
		Mask("phone", "13800138000"),
	)

	// 验证 Err(nil) 不输出空属性
	nilAttr := Err(nil)
	if nilAttr.Key != "" {
		t.Errorf("Err(nil) 预期返回空 Attr，实际 key 值为 %q", nilAttr.Key)
	}

	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("读取日志文件失败: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, `"str_key":"hello"`) {
		t.Errorf("预期包含 str_key 属性，实际内容: %s", content)
	}
	if !strings.Contains(content, `"phone":"138****8000"`) {
		t.Errorf("预期 phone 属性被脱敏为 138****8000，实际内容: %s", content)
	}
}

// TestLogValuer 测试原生 LogValuer 接口对敏感结构体的自动隐蔽与格式化
func TestLogValuer(t *testing.T) {
	ctx := context.Background()
	tmpDir := getTestTmpDir(t)
	logFile := filepath.Join(tmpDir, "test_valuer.log")

	logger := InitLogger(Config{
		Level:         "info",
		Format:        "json",
		EnableConsole: false,
		File:          FileConfig{Filename: logFile},
	})

	user := SensitiveUser{
		ID:         "U001",
		Name:       "John",
		Password:   "secret123",
		CreditCard: "1234567890123456",
	}

	logger.InfoAttrs(ctx, "用户登录事件", slog.Any("user", user))

	data, _ := os.ReadFile(logFile)
	content := string(data)

	if strings.Contains(content, "secret123") {
		t.Errorf("明文密码泄露在日志中！实际内容: %s", content)
	}
	if !strings.Contains(content, `"password":"******"`) {
		t.Errorf("预期密码被自动转化为 ******，实际内容: %s", content)
	}
}

// TestContextAttrs 测试通过 WithContextAttr 在 Context 中绑定公共属性并随日志带出
func TestContextAttrs(t *testing.T) {
	tmpDir := getTestTmpDir(t)
	logFile := filepath.Join(tmpDir, "test_ctx.log")

	logger := InitLogger(Config{
		Level:         "info",
		Format:        "json",
		EnableConsole: false,
		File:          FileConfig{Filename: logFile},
	})

	ctx := WithContextAttr(context.Background(), String("req_id", "REQ-100"), Int("tenant_id", 9))
	logger.InfoAttrs(ctx, "执行业务操作")

	data, _ := os.ReadFile(logFile)
	content := string(data)

	if !strings.Contains(content, `"req_id":"REQ-100"`) || !strings.Contains(content, `"tenant_id":9`) {
		t.Errorf("预期日志中带出 Context 绑定的属性，实际内容: %s", content)
	}
}

// TestAttachAndDetach 测试模块日志的 Attach（双写派生）与 Detach（隔离派生）行为
func TestAttachAndDetach(t *testing.T) {
	ctx := context.Background()
	tmpDir := getTestTmpDir(t)
	mainLog := filepath.Join(tmpDir, "main.log")

	logger := InitLogger(Config{
		Level:         "info",
		Format:        "json",
		EnableConsole: false,
		AttachLogDir:  tmpDir,
		File:          FileConfig{Filename: mainLog},
	})

	// 1. 测试 Attach
	attachLog := logger.Attach("order")
	attachLog.InfoAttrs(ctx, "订单创建完成")

	// 2. 测试 Detach
	detachLog := logger.Detach("audit")
	detachLog.InfoAttrs(ctx, "审计审计记录")

	time.Sleep(50 * time.Millisecond)

	mainData, _ := os.ReadFile(mainLog)
	mainContent := string(mainData)

	attachData, _ := os.ReadFile(filepath.Join(tmpDir, "order.log"))
	attachContent := string(attachData)

	detachData, _ := os.ReadFile(filepath.Join(tmpDir, "detach_audit.log"))
	detachContent := string(detachData)

	// 验证 Attach: 既存在于 mainLog，也存在于 order.log
	if !strings.Contains(mainContent, "订单创建完成") {
		t.Errorf("Attach 模式的日志应该打入 main.log")
	}
	if !strings.Contains(attachContent, "订单创建完成") {
		t.Errorf("Attach 模式的日志应该打入 order.log")
	}

	// 验证 Detach: 存在于 detach_audit.log，但绝不出现在 mainLog
	if strings.Contains(mainContent, "审计审计记录") {
		t.Errorf("Detach 隔离日志不应该出现在 main.log")
	}
	if !strings.Contains(detachContent, "审计审计记录") {
		t.Errorf("Detach 隔离日志必须打入 detach_audit.log")
	}
}

// TestToStdLogger 测试将 slog 桥接导出为标准库 *log.Logger 句柄
func TestToStdLogger(t *testing.T) {
	tmpDir := getTestTmpDir(t)
	logFile := filepath.Join(tmpDir, "test_std.log")

	logger := InitLogger(Config{
		Level:         "info",
		Format:        "json",
		EnableConsole: false,
		File:          FileConfig{Filename: logFile},
	})

	stdLog := logger.ToStdLogger(slog.LevelError)
	if stdLog == nil {
		t.Fatal("ToStdLogger 返回 nil")
	}

	stdLog.Println("来自第三方标准库调用的日志")

	data, _ := os.ReadFile(logFile)
	if !strings.Contains(string(data), "来自第三方标准库调用的日志") {
		t.Errorf("预期标准库 *log.Logger 写入日志成功，实际内容: %s", string(data))
	}
}

// TestSetLevel 测试动态日志级别的在线切换
func TestSetLevel(t *testing.T) {
	ctx := context.Background()
	tmpDir := getTestTmpDir(t)
	logFile := filepath.Join(tmpDir, "test_level.log")

	logger := InitLogger(Config{
		Level:         "info",
		Format:        "json",
		EnableConsole: false,
		File:          FileConfig{Filename: logFile},
	})

	logger.DebugAttrs(ctx, "不可见的调试日志")
	SetLevel("debug")
	logger.DebugAttrs(ctx, "调整级别后可见的调试日志")

	data, _ := os.ReadFile(logFile)
	content := string(data)

	if strings.Contains(content, "不可见的调试日志") {
		t.Errorf("修改日志级别前的 DEBUG 日志不应被写入文件")
	}
	if !strings.Contains(content, "调整级别后可见的调试日志") {
		t.Errorf("修改日志级别后的 DEBUG 日志应该被正常写入文件")
	}
}

// TestCatchPanic 测试 panic 崩溃拦截组件捕获并记录堆栈
func TestCatchPanic(t *testing.T) {
	ctx := context.Background()
	tmpDir := getTestTmpDir(t)
	logFile := filepath.Join(tmpDir, "test_panic.log")

	InitLogger(Config{
		Level:           "info",
		Format:          "json",
		EnableConsole:   false,
		PrintErrorStack: true,
		File:            FileConfig{Filename: logFile},
	})

	func() {
		defer CatchPanic(ctx)
		panic("单元测试故意触发的 panic")
	}()

	data, _ := os.ReadFile(logFile)
	content := string(data)

	if !strings.Contains(content, "[SYSTEM PANIC INTERCEPTED]") || !strings.Contains(content, "单元测试故意触发的 panic") {
		t.Errorf("预期 CatchPanic 拦截到异常并记录，实际内容: %s", content)
	}
}

// TestCustomOptions 测试通过 Option 模式传入自定义 Handler 和 ReplaceAttr 钩子
func TestCustomOptions(t *testing.T) {
	ctx := context.Background()
	tmpDir := getTestTmpDir(t)
	logFile := filepath.Join(tmpDir, "test_opt.log")
	customLogFile := filepath.Join(tmpDir, "test_custom_handler.log")

	customFileWriter := getLumberjackWriter(customLogFile, FileConfig{Filename: customLogFile})
	customHandler := slog.NewJSONHandler(customFileWriter, nil)

	logger := InitLogger(
		Config{
			Level:         "info",
			Format:        "json",
			EnableConsole: false,
			File:          FileConfig{Filename: logFile},
		},
		WithExtraHandler(customHandler),
		WithReplaceAttr(func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == "env" {
				return slog.String("env", "test-env")
			}
			return a
		}),
	)

	logger.InfoAttrs(ctx, "测试扩展 Handler", String("env", "raw"))

	time.Sleep(50 * time.Millisecond)

	mainData, _ := os.ReadFile(logFile)
	customData, _ := os.ReadFile(customLogFile)

	if !strings.Contains(string(mainData), `"env":"test-env"`) {
		t.Errorf("预期 ReplaceAttr 钩子成功替换属性，主日志内容: %s", string(mainData))
	}
	if !strings.Contains(string(customData), "测试扩展 Handler") {
		t.Errorf("预期自定义扩展 Handler 收到日志，自定义日志内容: %s", string(customData))
	}
}

// TestCustomTraceKey 测试自定义 TraceKey 以及 ContextExtractor
func TestCustomTraceKey(t *testing.T) {
	type customCtxKey string
	const myTraceKey customCtxKey = "X-My-Trace-Id"

	tmpDir := getTestTmpDir(t)
	logFile := filepath.Join(tmpDir, "test_trace.log")

	logger := InitLogger(
		Config{
			Level:         "info",
			Format:        "json",
			EnableConsole: false,
			File:          FileConfig{Filename: logFile},
		},
		WithTraceKey(myTraceKey, "my_trace_id"),
	)

	ctx := context.WithValue(context.Background(), myTraceKey, "TRACE-CUSTOM-999")
	logger.InfoAttrs(ctx, "测试自定义 TraceKey")

	data, _ := os.ReadFile(logFile)
	content := string(data)

	if !strings.Contains(content, `"my_trace_id":"TRACE-CUSTOM-999"`) {
		t.Errorf("预期日志包含自定义 trace_id，实际内容: %s", content)
	}
}

// TestSpecializedMasking 测试细粒度的专用脱敏函数
func TestSpecializedMasking(t *testing.T) {
	ctx := context.Background()
	tmpDir := getTestTmpDir(t)
	logFile := filepath.Join(tmpDir, "test_specialized_mask.log")

	logger := InitLogger(Config{
		Level:         "info",
		Format:        "json",
		EnableConsole: false,
		File:          FileConfig{Filename: logFile},
	})

	logger.InfoAttrs(ctx, "测试专有脱敏",
		MaskMobile("mobile", "13800138000"),
		MaskEmail("email", "zhangsan@domain.com"),
		MaskIDCard("id_card", "110101199003071234"),
		MaskBankCard("bank_card", "6222021204000001234"),
	)

	data, _ := os.ReadFile(logFile)
	content := string(data)

	if !strings.Contains(content, `"mobile":"138****8000"`) {
		t.Errorf("手机号脱敏失败: %s", content)
	}
	if !strings.Contains(content, `"email":"z***n@domain.com"`) {
		t.Errorf("邮箱脱敏失败: %s", content)
	}
	if !strings.Contains(content, `"id_card":"110101******1234"`) {
		t.Errorf("身份证号脱敏失败: %s", content)
	}
	if !strings.Contains(content, `"bank_card":"622202******1234"`) {
		t.Errorf("银行卡号脱敏失败: %s", content)
	}
}

// TestConfigValidation 测试配置校验与容错兜底
func TestConfigValidation(t *testing.T) {
	invalidCfg := Config{
		Level:         "invalid_level",
		EnableConsole: false,
	}

	err := invalidCfg.Validate()
	if err == nil {
		t.Error("预期非法 Level 返回校验错误")
	}

	noTargetCfg := Config{
		EnableConsole: false,
	}
	err = noTargetCfg.Validate()
	if !errors.Is(err, ErrNoLogOutputTarget) {
		t.Errorf("预期无输出目标返回 ErrNoLogOutputTarget，实际: %v", err)
	}
}

// TestErrorFilename Separate error log file testing
func TestErrorFilename(t *testing.T) {
	ctx := context.Background()
	tmpDir := getTestTmpDir(t)
	mainLog := filepath.Join(tmpDir, "main.log")
	errLog := filepath.Join(tmpDir, "error.log")

	logger := InitLogger(Config{
		Level:         "info",
		Format:        "json",
		EnableConsole: false,
		File:          FileConfig{Filename: mainLog},
		ErrorFilename: errLog,
	})

	logger.InfoAttrs(ctx, "正常信息")
	logger.ErrorAttrs(ctx, "异常报错", errors.New("something went wrong"))

	time.Sleep(50 * time.Millisecond)

	mainData, _ := os.ReadFile(mainLog)
	errData, _ := os.ReadFile(errLog)

	if !strings.Contains(string(mainData), "正常信息") || !strings.Contains(string(mainData), "异常报错") {
		t.Errorf("主日志应该包含 Info 和 Error: %s", string(mainData))
	}
	if strings.Contains(string(errData), "正常信息") {
		t.Errorf("独立 Error 日志不应该包含 Info: %s", string(errData))
	}
	if !strings.Contains(string(errData), "异常报错") {
		t.Errorf("独立 Error 日志应该包含 Error: %s", string(errData))
	}
}
