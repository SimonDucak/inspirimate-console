// Package notifymail sends plain-text notices through an operator SMTP server.
// The password is used only for authentication and is never written into a message.
package notifymail

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"strings"
	"time"
	"unicode/utf8"
)

// Config is the instance SMTP account. Empty Host disables sending.
type Config struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
}

func (c *Config) Enabled() bool {
	return c != nil && c.Host != "" && c.From != "" && c.Port > 0
}

// Send delivers one plain-text message. to must already be validated addresses.
func (c *Config) Send(ctx context.Context, to []string, subject, body string) error {
	if !c.Enabled() {
		return errors.New("smtp is not configured")
	}
	if len(to) == 0 {
		return errors.New("no recipients")
	}
	addr := net.JoinHostPort(c.Host, fmt.Sprintf("%d", c.Port))
	dialer := net.Dialer{Timeout: 15 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return errors.New("could not reach the mail server")
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	client, err := smtp.NewClient(conn, c.Host)
	if err != nil {
		return errors.New("could not start the mail session")
	}
	defer client.Close()
	ehlo := "localhost"
	if at := strings.LastIndex(c.From, "@"); at > 0 && at < len(c.From)-1 {
		ehlo = c.From[at+1:]
	}
	if err := client.Hello(ehlo); err != nil {
		return errors.New("mail server rejected the greeting")
	}
	if !isLocal(c.Host) {
		if err := client.StartTLS(&tls.Config{ServerName: c.Host}); err != nil {
			return errors.New("mail server did not start a secure connection")
		}
	}
	if c.Username != "" {
		if err := client.Auth(smtp.PlainAuth("", c.Username, c.Password, c.Host)); err != nil {
			return errors.New("mail server rejected the login")
		}
	}
	if err := client.Mail(c.From); err != nil {
		return errors.New("mail server rejected the sender")
	}
	for _, rcpt := range to {
		if err := client.Rcpt(rcpt); err != nil {
			return errors.New("mail server rejected a recipient")
		}
	}
	w, err := client.Data()
	if err != nil {
		return errors.New("mail server rejected the message")
	}
	if _, err := w.Write([]byte(message(c.From, to, subject, body))); err != nil {
		_ = w.Close()
		return errors.New("could not send the message")
	}
	if err := w.Close(); err != nil {
		return errors.New("mail server did not accept the message")
	}
	return client.Quit()
}

func isLocal(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func message(from string, to []string, subject, body string) string {
	subject = strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' {
			return -1
		}
		return r
	}, subject)
	if !isASCII(subject) {
		subject = mime.QEncoding.Encode("utf-8", subject)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", strings.Join(to, ", "))
	fmt.Fprintf(&b, "Subject: %s\r\n", subject)
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(strings.ReplaceAll(body, "\n", "\r\n"))
	return b.String()
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

// TaskNotice is the message sent when a coding agent reaches a terminal state.
func TaskNotice(appName, status, prompt, projectURL string) (subject, body string) {
	name := strings.TrimSpace(appName)
	if name == "" {
		name = "A project"
	}
	verb := "finished"
	if status != "succeeded" {
		verb = "stopped"
	}
	subject = "Coding " + verb + ": " + name
	var b strings.Builder
	fmt.Fprintf(&b, "The coding agent %s on %s.\n", verb, name)
	if status != "" && status != "succeeded" {
		fmt.Fprintf(&b, "Status: %s\n", status)
	}
	if excerpt := excerpt(prompt); excerpt != "" {
		fmt.Fprintf(&b, "\nRequest:\n%s\n", excerpt)
	}
	if projectURL != "" {
		fmt.Fprintf(&b, "\nOpen the project:\n%s\n", projectURL)
	}
	return subject, b.String()
}

func excerpt(prompt string) string {
	prompt = strings.Join(strings.Fields(prompt), " ")
	if prompt == "" {
		return ""
	}
	const max = 400
	if utf8.RuneCountInString(prompt) <= max {
		return prompt
	}
	i := 0
	for n := 0; n < max && i < len(prompt); n++ {
		_, size := utf8.DecodeRuneInString(prompt[i:])
		i += size
	}
	return prompt[:i] + "…"
}
