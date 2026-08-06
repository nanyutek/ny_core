package nylog

import "log/slog"

// --- 强类型 Attr 构建器 (替代裸字符串, 彻底规避 !BADKEY 陷阱) ---

func String(key, val string) slog.Attr      { return slog.String(key, val) }
func Int(key string, val int) slog.Attr     { return slog.Int(key, val) }
func Int64(key string, val int64) slog.Attr { return slog.Int64(key, val) }
func Bool(key string, val bool) slog.Attr   { return slog.Bool(key, val) }

func Group(key string, attrs ...slog.Attr) slog.Attr {
	args := make([]any, len(attrs))
	for i, attr := range attrs {
		args[i] = attr
	}
	return slog.Group(key, args...)
}

// Err 安全包装 Error (处理 err == nil 场景，避免输出空字符串)
func Err(err error) slog.Attr {
	if err == nil {
		return slog.Attr{}
	}
	return slog.String("error", err.Error())
}

// Mask 脱敏助手 (如 Phone: 138****1234, Email: a***@qq.com)
func Mask(key, val string) slog.Attr {
	if len(val) <= 4 {
		return slog.String(key, "****")
	}
	return slog.String(key, val[:3]+"****"+val[len(val)-4:])
}

// SensitiveUser 结构体自动脱敏示范 (实现 slog.LogValuer 接口)
type SensitiveUser struct {
	ID         string
	Name       string
	Password   string
	CreditCard string
}

func (u SensitiveUser) LogValue() slog.Value {
	cc := u.CreditCard
	if len(cc) > 4 {
		cc = "****-****-****-" + cc[len(cc)-4:]
	}
	return slog.GroupValue(
		slog.String("id", u.ID),
		slog.String("name", u.Name),
		slog.String("password", "******"),
		slog.String("credit_card", cc),
	)
}
