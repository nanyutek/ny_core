package nylog

import "log/slog"

type FileFormat string

const (
	FileFormatJSON FileFormat = "json"
	FileFormatText FileFormat = "text"
)

// Config 主日志配置结构
type Config struct {
	Level           string     `json:"level" yaml:"level"`                         // debug, info, warn, error
	Format          FileFormat `json:"format" yaml:"format"`                       // json 或 text
	EnableConsole   bool       `json:"enable_console" yaml:"enable_console"`       // 是否输出到控制台
	PrintErrorStack bool       `json:"print_error_stack" yaml:"print_error_stack"` // Error 级别是否打堆栈
	File            FileConfig `json:"file" yaml:"file"`                           // 主日志文件配置
	AttachLogDir    string     `json:"attach_log_dir" yaml:"attach_log_dir"`       // 附加/隔离模块日志存储目录
}

// FileConfig 单个日志文件切割归档配置
type FileConfig struct {
	Filename   string `json:"filename" yaml:"filename"`         // 主日志文件名，如 logs/app.log
	MaxSizeMB  int    `json:"max_size_mb" yaml:"max_size_mb"`   // 单文件最大MB
	MaxBackups int    `json:"max_backups" yaml:"max_backups"`   // 最大留存份数
	MaxAgeDays int    `json:"max_age_days" yaml:"max_age_days"` // 最大留存天数
	Compress   bool   `json:"compress" yaml:"compress"`         // 是否 Gzip 压缩
}

// DefaultConfig 默认生产推荐配置
func DefaultConfig() Config {
	return Config{
		Level:           "info",
		Format:          FileFormatJSON,
		EnableConsole:   true,
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

func parseLevel(lvl string) slog.Level {
	switch lvl {
	case "debug", "DEBUG":
		return slog.LevelDebug
	case "info", "INFO":
		return slog.LevelInfo
	case "warn", "WARN":
		return slog.LevelWarn
	case "error", "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
