// Package mailer sends the one-time verification code to a tester's email. It
// is an interface with two implementations so local development needs no email
// infrastructure: LogMailer prints the code to the service log, while Resend
// sends real mail in production.
package mailer

import "context"

// Mailer delivers a one-time code to an email address.
type Mailer interface {
	SendOTP(ctx context.Context, toEmail, code string) error
}
