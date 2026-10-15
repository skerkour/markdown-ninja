package smtp

import (
	"context"

	"markdown.ninja/stdx/email"
)

// Mailer implements the `email.Mailer` interface to send emails using SMTP
type Mailer struct {
	smtpMailer email.SMTPMailer
}

// NewMailer returns a new smtp Mailer
func NewMailer(config email.SMTPConfig) *Mailer {
	return &Mailer{
		smtpMailer: email.NewSMTPMailer(config),
	}
}

// Send an email using the SMTP mailer
func (mailer *Mailer) SendTransactionnal(ctx context.Context, email email.Email) error {
	return mailer.smtpMailer.Send(email)
}

// TODO
func (mailer *Mailer) SendBroadcast(ctx context.Context, email email.Email) error {
	return mailer.smtpMailer.Send(email)
}
