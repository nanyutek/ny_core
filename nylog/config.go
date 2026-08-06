package nylog

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
)

type FileFormat string

const (
	FileFormatJSON FileFormat = "json"
	FileFormatText FileFormat = "text"
)

var ErrNoLogOutputTarget = errors.New("nylog: no valid log output target enabled (console is disabled and filenames are empty)")

// Config 主日志配置结构
type Config struct {
	Level           string     `json:"level" yaml:"level"`                         // debug, info, warn, error
	Format          FileFormat `json:"format" yaml:"format"`                       // json 或 text
	EnableConsole   bool       `json:"enable_console" yaml:"enable_console"`       // 是否输出到控制台
	EnableAddSource *bool      `json:"enable_add_source" yaml:"enable_add_source"` // 是否记录代码行号 (默认 true)
	PrintErrorStack bool       `json:"print_error_stack" yaml:"print_error_stack"` // Error 级别是否打印堆栈
	File            FileConfig `json:"file" yaml:"file"`                           // 主日志文件配置
	ErrorFilename   string     `json:"error_filename" yaml:"error_filename"`       // 可选：独立的 Error 级别日志文件 (如 logs/error.log)
	AttachLogDir    string     `json:"attach_log_dir" yaml:"attach_log_dir"`       // 附加/隔离模块日志存储目录
}

// FileConfig 单个日志文件切割归档配置
type FileConfig struct {
	Filename   string `json:"filename" yaml:"filename"`         // 主日志文件名，如 logs/app.log
	MaxSizeMB  int    `json:"max_size_mb" yaml:"max_size_mb"`   // 单文件最大MB (<=0 默认100MB)
	MaxBackups int    `json:"max_backups" yaml:"max_backups"`   // 最大留存份数 (<=0 不限份数)
	MaxAgeDays int    `json:"max_age_days" yaml:"max_age_days"` // 最大留存天数 (<=0 不限天数)
	Compress   bool   `json:"compress" yaml:"compress"`         // 是否 Gzip 压缩
}

// DefaultConfig 默认生产推荐配置
func DefaultConfig() Config {
	addSource := true
	return Config{
		Level:           "info",
		Format:          FileFormatJSON,
		EnableConsole:   true,
		EnableAddSource: &addSource,
		PrintErrorStack: true,
		AttachLogDir:    "logs",
		File: FileConfig{
			Filename:   "logs/app.log",
			MaxSizeMB:  100,
			MaxBackups: 10,
			MaxAgeDays: 30,
			Compress:   true,
		},
	}
}

// Validate 校验并规范化配置，发现致命误配置时返回 error
func (c *Config) Validate() error {
	c.Level = strings.TrimSpace(strings.ToLower(c.Level))
	switch c.Level {
	case "debug", "info", "warn", "error":
	case "":
		c.Level = "info"
	default:
		return fmt.Errorf("nylog: invalid log level %q (expected debug, info, warn, error)", c.Level)
	}

	if c.Format == "" {
		c.Format = FileFormatJSON
	} else if c.Format != FileFormatJSON && c.Format != FileFormatText {
		return fmt.Errorf("nylog: invalid log format %q (expected json or text)", c.Format)
	}

	if !c.EnableConsole && c.File.Filename == "" && c.ErrorFilename == "" {
		return ErrNoLogOutputTarget
	}

	if c.File.Filename != "" && c.File.MaxSizeMB <= 0 {
		c.File.MaxSizeMB = 100 // 默认切割大小兜底
	}

	if c.AttachLogDir == "" {
		c.AttachLogDir = "logs"
	}

	return nil
}

func (c *Config) IsAddSourceEnabled() bool {
	if c.EnableAddSource == nil {
		return true
	}
	return *c.EnableAddSource
}

func parseLevel(lvl string) slog.Level {
	switch strings.ToLower(lvl) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
