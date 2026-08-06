package nylog

import (
	"log/slog"
	"strings"
)

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

// --- 常用敏感数据脱敏助手函数 ---

// Mask 通用保底脱敏 (保留前3后4)
func Mask(key, val string) slog.Attr {
	if len(val) <= 4 {
		return slog.String(key, "****")
	}
	if len(val) <= 7 {
		return slog.String(key, val[:2]+"****"+val[len(val)-1:])
	}
	return slog.String(key, val[:3]+"****"+val[len(val)-4:])
}

// MaskMobile 手机号脱敏 (例: 138****1234)
func MaskMobile(key, mobile string) slog.Attr {
	if len(mobile) != 11 {
		return Mask(key, mobile)
	}
	return slog.String(key, mobile[:3]+"****"+mobile[7:])
}

// MaskEmail 邮箱脱敏 (例: a***@domain.com)
func MaskEmail(key, email string) slog.Attr {
	atIdx := strings.Index(email, "@")
	if atIdx <= 1 {
		return slog.String(key, "*@*")
	}
	name := email[:atIdx]
	domain := email[atIdx:]
	if len(name) <= 2 {
		return slog.String(key, name[:1]+"***"+domain)
	}
	return slog.String(key, name[:1]+"***"+name[len(name)-1:]+domain)
}

// MaskIDCard 身份证号脱敏 (例: 110101******1234)
func MaskIDCard(key, idCard string) slog.Attr {
	if len(idCard) != 18 {
		return Mask(key, idCard)
	}
	return slog.String(key, idCard[:6]+"******"+idCard[14:])
}

// MaskBankCard 银行卡号脱敏 (例: 622202******1234)
func MaskBankCard(key, cardNo string) slog.Attr {
	if len(cardNo) < 12 {
		return Mask(key, cardNo)
	}
	return slog.String(key, cardNo[:6]+"******"+cardNo[len(cardNo)-4:])
}

// SensitiveUser 结构体自动脱敏示范 (实现 slog.LogValuer 接口)
type SensitiveUser struct {
	ID         string
	Name       string
	Password   string
	CreditCard string
}

func (u SensitiveUser) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("id", u.ID),
		slog.String("name", u.Name),
		slog.String("password", "******"),
		slog.String("credit_card", MaskBankCard("cc", u.CreditCard).Value.String()),
	)
}
