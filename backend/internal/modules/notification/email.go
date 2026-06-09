package notification

import (
	"context"
	"errors"
	"fmt"
	"net/smtp"
	"strings"
	"sync"
	"sync/atomic"
)

var smtpBoundaryCounter atomic.Int64

// EmailSender abstracts SMTP for testing.
type EmailSender interface {
	Send(ctx context.Context, to, subject, htmlBody, textBody string) error
}

// MockSender records sent emails for tests.
type MockSender struct {
	mu   sync.Mutex
	Sent []EmailLog
	Fail bool
}

type EmailLog struct {
	To, Subject, HTMLBody, TextBody string
}

func (m *MockSender) Send(_ context.Context, to, subject, html, text string) error {
	if m.Fail {
		return errors.New("mock smtp failure")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Sent = append(m.Sent, EmailLog{to, subject, html, text})
	return nil
}

// SMTPSender sends via net/smtp with multipart/alternative MIME.
type SMTPSender struct {
	host     string
	port     string
	user     string
	pass     string
	fromAddr string
	fromName string
}

func NewSMTPSender(host, port, user, pass, fromAddr, fromName string) *SMTPSender {
	return &SMTPSender{host: host, port: port, user: user, pass: pass, fromAddr: fromAddr, fromName: fromName}
}

func (s *SMTPSender) Send(_ context.Context, to, subject, htmlBody, textBody string) error {
	boundary := fmt.Sprintf("vo_boundary_%d", smtpBoundaryCounter.Add(1))
	msg := strings.Builder{}
	msg.WriteString("From: " + s.fromName + " <" + s.fromAddr + ">\r\n")
	msg.WriteString("To: " + to + "\r\n")
	msg.WriteString("Subject: " + subject + "\r\n")
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString("Content-Type: multipart/alternative; boundary=\"" + boundary + "\"\r\n")
	msg.WriteString("\r\n")
	msg.WriteString("--" + boundary + "\r\n")
	msg.WriteString("Content-Type: text/plain; charset=UTF-8\r\n\r\n")
	msg.WriteString(textBody + "\r\n")
	msg.WriteString("--" + boundary + "\r\n")
	msg.WriteString("Content-Type: text/html; charset=UTF-8\r\n\r\n")
	msg.WriteString("<html><body>" + htmlBody + "</body></html>\r\n")
	msg.WriteString("--" + boundary + "--\r\n")

	auth := smtp.PlainAuth("", s.user, s.pass, s.host)
	return smtp.SendMail(s.host+":"+s.port, auth, s.fromAddr, []string{to}, []byte(msg.String()))
}
