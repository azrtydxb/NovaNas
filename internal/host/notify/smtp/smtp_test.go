package smtp

import (
	"bufio"
	"context"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeSMTPServer implements just enough of RFC 5321 to receive a single
// message over a plaintext connection (no STARTTLS, no AUTH). It records
// the SMTP transcript and returns the DATA payload to the caller.
type fakeSMTPServer struct {
	mu          sync.Mutex
	mailFrom    string
	rcpts       []string
	data        string
	conversation []string
	requireAuth bool
}

func (f *fakeSMTPServer) handle(c net.Conn) {
	defer c.Close()
	r := bufio.NewReader(c)
	w := bufio.NewWriter(c)
	write := func(s string) {
		w.WriteString(s)
		w.WriteString("\r\n")
		w.Flush()
	}
	write("220 fake.smtp ESMTP")
	inData := false
	var dataBuf strings.Builder
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		f.mu.Lock()
		f.conversation = append(f.conversation, line)
		f.mu.Unlock()
		if inData {
			if line == "." {
				inData = false
				f.mu.Lock()
				f.data = dataBuf.String()
				f.mu.Unlock()
				write("250 OK queued")
				continue
			}
			if strings.HasPrefix(line, ".") {
				line = line[1:]
			}
			dataBuf.WriteString(line)
			dataBuf.WriteString("\r\n")
			continue
		}
		up := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(up, "EHLO"), strings.HasPrefix(up, "HELO"):
			write("250-fake.smtp")
			write("250 SIZE 10485760")
		case strings.HasPrefix(up, "MAIL FROM:"):
			f.mu.Lock()
			f.mailFrom = strings.TrimSpace(line[len("MAIL FROM:"):])
			f.mu.Unlock()
			write("250 OK")
		case strings.HasPrefix(up, "RCPT TO:"):
			f.mu.Lock()
			f.rcpts = append(f.rcpts, strings.TrimSpace(line[len("RCPT TO:"):]))
			f.mu.Unlock()
			write("250 OK")
		case up == "DATA":
			write("354 End data with <CR><LF>.<CR><LF>")
			inData = true
		case up == "QUIT":
			write("221 Bye")
			return
		case up == "RSET":
			write("250 OK")
		default:
			write("250 OK")
		}
	}
}

func startFakeServer(t *testing.T) (addr string, srv *fakeSMTPServer, stop func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv = &fakeSMTPServer{}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go srv.handle(c)
		}
	}()
	return ln.Addr().String(), srv, func() { ln.Close() }
}

func TestSend_Plaintext(t *testing.T) {
	addr, srv, stop := startFakeServer(t)
	defer stop()

	host, portStr, _ := net.SplitHostPort(addr)
	var port int
	for _, b := range []byte(portStr) {
		port = port*10 + int(b-'0')
	}

	c, err := New(Config{
		Host:        host,
		Port:        port,
		FromAddress: "alerts@novanas.local",
		TLSMode:     TLSModeNone,
		Timeout:     5 * time.Second,
	})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if err := c.Send(context.Background(), []string{"ops@example.com"}, "hello", "body line\n.dotline\nlast", map[string]string{"X-Trace": "abc"}); err != nil {
		t.Fatalf("send: %v", err)
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if !strings.Contains(srv.mailFrom, "alerts@novanas.local") {
		t.Errorf("MAIL FROM=%q", srv.mailFrom)
	}
	if len(srv.rcpts) != 1 || !strings.Contains(srv.rcpts[0], "ops@example.com") {
		t.Errorf("rcpts=%v", srv.rcpts)
	}
	if !strings.Contains(srv.data, "Subject: hello") {
		t.Errorf("Subject missing: %q", srv.data)
	}
	if !strings.Contains(srv.data, "X-Trace: abc") {
		t.Errorf("X-Trace missing")
	}
	if !strings.Contains(srv.data, "body line") || !strings.Contains(srv.data, ".dotline") {
		t.Errorf("body missing/incorrect: %q", srv.data)
	}
	if strings.Contains(srv.data, "Subject: hello\nFrom") {
		t.Errorf("CRLF normalization failed")
	}
}

func TestNew_ValidatesConfig(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
	}{
		{"missing host", Config{Port: 25, FromAddress: "a@b"}},
		{"bad port", Config{Host: "h", Port: 0, FromAddress: "a@b"}},
		{"missing from", Config{Host: "h", Port: 25}},
		{"bad mode", Config{Host: "h", Port: 25, FromAddress: "a@b", TLSMode: TLSMode("weird")}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := New(tc.cfg); err == nil {
				t.Fatalf("expected error")
			}
		})
	}
}

func TestSend_RejectsEmptyRecipients(t *testing.T) {
	c, _ := New(Config{Host: "h", Port: 25, FromAddress: "a@b", TLSMode: TLSModeNone})
	if err := c.Send(context.Background(), nil, "s", "b", nil); err == nil {
		t.Fatalf("expected error")
	}
}

func TestBuildMessage_StripsHeaderInjection(t *testing.T) {
	msg := buildMessage("a@b", []string{"c@d"}, "evil\r\nBcc: x@y", "body", nil)
	// CRLF must be folded into the subject; there must be no real Bcc header.
	if strings.Contains(msg, "\nBcc:") || strings.Contains(msg, "\r\nBcc:") {
		t.Fatalf("CRLF in subject leaked Bcc header: %q", msg)
	}
	if !strings.Contains(msg, "Subject: evil") {
		t.Fatalf("subject missing: %q", msg)
	}
}

func TestBuildMessage_HeadersDoNotOverrideFromTo(t *testing.T) {
	msg := buildMessage("a@b", []string{"c@d"}, "s", "body", map[string]string{
		"From": "evil@x", "To": "evil@y", "X-Custom": "ok",
	})
	if strings.Count(msg, "From: a@b") != 1 || strings.Contains(msg, "evil@x") {
		t.Fatalf("From override leaked: %q", msg)
	}
	if !strings.Contains(msg, "X-Custom: ok") {
		t.Fatalf("custom header missing")
	}
}
