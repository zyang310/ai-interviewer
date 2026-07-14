package mailer

import (
	"context"
	"log"
)

// LogMailer prints the OTP to the service log instead of emailing it. It is the
// default local/dev mailer: the whole activation flow is testable end-to-end
// with no domain, no Resend account, and no deliverability concerns — read the
// code off the log and type it into the app.
type LogMailer struct{}

// NewLog returns a LogMailer.
func NewLog() *LogMailer { return &LogMailer{} }

var _ Mailer = (*LogMailer)(nil)

// SendOTP logs the code. The log line is intentionally greppable in dev.
func (LogMailer) SendOTP(_ context.Context, toEmail, code string) error {
	log.Printf("mailer(log): OTP for %s: %s", toEmail, code)
	return nil
}
