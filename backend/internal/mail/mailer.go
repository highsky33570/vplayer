package mail

import (
	"context"
	"fmt"
	"strings"
)

// Message is a provider-neutral outbound email.
type Message struct {
	To       string
	Subject  string
	TextBody string
	HTMLBody string
}

// Mailer sends transactional email. Implementations must not log secrets or codes.
type Mailer interface {
	Send(ctx context.Context, msg Message) error
}

// VerificationEmail builds a simple VPlayer registration verification message.
// code is included only in the message bodies (caller must not log it).
func VerificationEmail(to, code string) Message {
	to = strings.TrimSpace(to)
	text := fmt.Sprintf(
		"天涯欧乐 注册验证\n\n您的验证码是：%s\n\n验证码 10 分钟内有效。\n如非本人操作，请忽略本邮件。\n",
		code,
	)
	html := fmt.Sprintf(
		`<p>天涯欧乐 注册验证</p><p>您的验证码是：<strong style="font-size:20px;letter-spacing:2px">%s</strong></p><p>验证码 10 分钟内有效。</p><p>如非本人操作，请忽略本邮件。</p>`,
		code,
	)
	return Message{
		To:       to,
		Subject:  "天涯欧乐 注册验证码",
		TextBody: text,
		HTMLBody: html,
	}
}

// PasswordResetEmail builds a forgot-password verification message.
// code is included only in the message bodies (caller must not log it).
func PasswordResetEmail(to, code string) Message {
	to = strings.TrimSpace(to)
	text := fmt.Sprintf(
		"天涯欧乐 找回密码\n\n您的验证码是：%s\n\n验证码 10 分钟内有效。\n如非本人操作，请忽略本邮件。\n",
		code,
	)
	html := fmt.Sprintf(
		`<p>天涯欧乐 找回密码</p><p>您的验证码是：<strong style="font-size:20px;letter-spacing:2px">%s</strong></p><p>验证码 10 分钟内有效。</p><p>如非本人操作，请忽略本邮件。</p>`,
		code,
	)
	return Message{
		To:       to,
		Subject:  "天涯欧乐 找回密码验证码",
		TextBody: text,
		HTMLBody: html,
	}
}
