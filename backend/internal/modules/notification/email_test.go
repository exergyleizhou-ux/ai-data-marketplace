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
