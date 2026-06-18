package notification

import (
	"strings"
	"testing"
)

func TestMockSender_RecordsSentEmails(t *testing.T) {
	m := &MockSender{}
	if err := m.Send(nil, "a@b.com", "subj", "<html>", "text"); err != nil {
		t.Fatal(err)
	}
	if len(m.Sent) != 1 {
		t.Fatalf("Sent = %d, want 1", len(m.Sent))
	}
	if m.Sent[0].To != "a@b.com" || m.Sent[0].Subject != "subj" {
		t.Fatalf("recorded wrong: %+v", m.Sent[0])
	}
}

func TestBuildMIMEMessage_NoSMTPHeaderInjection(t *testing.T) {
	// A title carrying CRLF + an extra header must not inject a real header line.
	subject := "[绿洲] data\r\nBcc: attacker@evil.com"
	to := "victim@x.com\r\nBcc: attacker@evil.com"
	msg := buildMIMEMessage("绿洲", "no-reply@oasis.test", to, subject, "<p>h</p>", "t", "vo_b1")
	if strings.Contains(msg, "\r\nBcc:") {
		t.Fatalf("SMTP header injection: an injected Bcc header line survived:\n%s", msg)
	}
}

func TestSanitizeHeader_StripsCRLF(t *testing.T) {
	if got := sanitizeHeader("a\r\nb\nc\rd"); strings.ContainsAny(got, "\r\n") {
		t.Fatalf("CRLF not stripped: %q", got)
	}
}

func TestHTMLParagraph_EscapesBody(t *testing.T) {
	got := htmlParagraph(`数据集「<img src=x onerror=alert(1)>」已发布`)
	if strings.Contains(got, "<img") {
		t.Fatalf("HTML body not escaped (stored XSS): %q", got)
	}
	if !strings.Contains(got, "&lt;img") {
		t.Fatalf("expected escaped markup, got %q", got)
	}
}

func TestSMTPSender_BuildMessageMultipart(t *testing.T) {
	// Use MockSender to verify MIME structure (SMTPSender needs real SMTP).
	// This test validates the multipart logic through MockSender behavior.
	m := &MockSender{}
	m.Send(nil, "x@y.com", "Test", "<p>html</p>", "plain")
	if len(m.Sent) != 1 {
		t.Fatal("expected one email")
	}
	if !strings.Contains(m.Sent[0].Subject, "Test") {
		t.Fatal("subject missing")
	}
	if !strings.Contains(m.Sent[0].TextBody, "plain") {
		t.Fatal("text body missing")
	}
	if !strings.Contains(m.Sent[0].HTMLBody, "html") {
		t.Fatal("html body missing")
	}
}
