package mailer

import "embed"

const (
	FromName              = "Khel"
	maxRetires            = 3
	UserWelcomeTemplate   = "user_invitation.tmpl"
	ResetPasswordTemplate = "reset_password.tmpl"
)

//go:embed "templates"
var FS embed.FS

type Client interface {
	Send(templateFile, username, email string, data any) (int, error)
}
