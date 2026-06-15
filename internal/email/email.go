package email

import (
	"fmt"
	"net/smtp"
	"shopping/internal/config"
)

type Mailer struct {
	cfg *config.Config
}

func New(cfg *config.Config) *Mailer {
	return &Mailer{cfg: cfg}
}

func (m *Mailer) SendInvite(to, inviteURL, fromName string) error {
	subject := fmt.Sprintf("%s invited you to the shopping list", fromName)
	body := fmt.Sprintf(
		"Hi,\r\n\r\n"+
			"%s has invited you to join the shared shopping list.\r\n\r\n"+
			"Click the link below to create your account (link expires in 48 hours):\r\n\r\n"+
			"%s\r\n\r\n"+
			"If you did not expect this invitation, you can ignore this email.\r\n",
		fromName, inviteURL,
	)

	msg := fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
		m.cfg.SMTPFrom, to, subject, body,
	)

	addr := fmt.Sprintf("%s:%d", m.cfg.SMTPHost, m.cfg.SMTPPort)

	var auth smtp.Auth
	if m.cfg.SMTPUser != "" {
		auth = smtp.PlainAuth("", m.cfg.SMTPUser, m.cfg.SMTPPass, m.cfg.SMTPHost)
	}

	return smtp.SendMail(addr, auth, m.cfg.SMTPFrom, []string{to}, []byte(msg))
}
