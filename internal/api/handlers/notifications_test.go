package handlers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	smtpmgr "github.com/novanas/nova-nas/internal/host/notify/smtp"
)

// fakeSMTP is a minimal RFC5321 plaintext server for tests.
type fakeSMTP struct {
	mu       sync.Mutex
	received string
	addr     string
	port     int
	stop     func()
}

func newFakeSMTP(t *testing.T) *fakeSMTP {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	tcp := ln.Addr().(*net.TCPAddr)
	f := &fakeSMTP{addr: tcp.IP.String(), port: tcp.Port, stop: func() { ln.Close() }}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go f.serve(c)
		}
	}()
	return f
}

func (f *fakeSMTP) serve(c net.Conn) {
	defer c.Close()
	r := bufio.NewReader(c)
	w := bufio.NewWriter(c)
	put := func(s string) { w.WriteString(s + "\r\n"); w.Flush() }
	put("220 fake")
	inData := false
	var buf bytes.Buffer
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		if inData {
			if line == "." {
				f.mu.Lock()
				f.received = buf.String()
				f.mu.Unlock()
				inData = false
				put("250 OK")
				continue
			}
			if strings.HasPrefix(line, ".") {
				line = line[1:]
			}
			buf.WriteString(line)
			buf.WriteString("\n")
			continue
		}
		up := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(up, "EHLO"), strings.HasPrefix(up, "HELO"):
			put("250-fake")
			put("250 SIZE 10485760")
		case strings.HasPrefix(up, "MAIL"), strings.HasPrefix(up, "RCPT"):
			put("250 OK")
		case up == "DATA":
			put("354 send")
			inData = true
		case up == "QUIT":
			put("221 bye")
			return
		default:
			put("250 OK")
		}
	}
}

func newTestMgr(t *testing.T, host string, port int) *smtpmgr.Manager {
	t.Helper()
	m, err := smtpmgr.NewManager(smtpmgr.Config{
		Host:        host,
		Port:        port,
		FromAddress: "alerts@novanas.local",
		TLSMode:     smtpmgr.TLSModeNone,
		Timeout:     5 * time.Second,
	}, 30)
	if err != nil {
		t.Fatalf("manager: %v", err)
	}
	return m
}

func TestNotifications_GetConfig_RedactsPassword(t *testing.T) {
	mgr, _ := smtpmgr.NewManager(smtpmgr.Config{
		Host: "smtp.example.com", Port: 587,
		Username: "user", Password: "secret",
		FromAddress: "x@y.z", TLSMode: smtpmgr.TLSModeStartTLS,
	}, 30)
	h := &NotificationsHandler{Logger: newDiscardLogger(), Mgr: mgr}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/notifications/smtp", nil)
	rr := httptest.NewRecorder()
	h.GetConfig(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var out SMTPConfigDTO
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Password != "***" {
		t.Errorf("password not redacted: %q", out.Password)
	}
	if out.Host != "smtp.example.com" {
		t.Errorf("host=%q", out.Host)
	}
}

func TestNotifications_PutConfig_PreservesPasswordOnRedaction(t *testing.T) {
	mgr, _ := smtpmgr.NewManager(smtpmgr.Config{
		Host: "old.example.com", Port: 25,
		Password: "kept", FromAddress: "x@y.z", TLSMode: smtpmgr.TLSModeNone,
	}, 30)
	h := &NotificationsHandler{Logger: newDiscardLogger(), Mgr: mgr}
	body := `{"host":"new.example.com","port":587,"fromAddress":"x@y.z","tlsMode":"starttls","password":"***"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/notifications/smtp", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.PutConfig(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := mgr.Config().Password; got != "kept" {
		t.Errorf("password not preserved: %q", got)
	}
	if got := mgr.Config().Host; got != "new.example.com" {
		t.Errorf("host not updated: %q", got)
	}
}

func TestNotifications_PutConfig_RejectsBadAddress(t *testing.T) {
	mgr, _ := smtpmgr.NewManager(smtpmgr.Config{}, 30)
	h := &NotificationsHandler{Logger: newDiscardLogger(), Mgr: mgr}
	body := `{"host":"h","port":25,"fromAddress":"not an email","tlsMode":"none"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/notifications/smtp", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.PutConfig(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestNotifications_PostTest_SendsEmail(t *testing.T) {
	srv := newFakeSMTP(t)
	defer srv.stop()
	mgr := newTestMgr(t, srv.addr, srv.port)
	h := &NotificationsHandler{Logger: newDiscardLogger(), Mgr: mgr}

	body := `{"to":"ops@example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/smtp/test", strings.NewReader(body)).WithContext(context.Background())
	rr := httptest.NewRecorder()
	h.PostTest(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if !strings.Contains(srv.received, "Subject: NovaNAS SMTP test") {
		t.Errorf("subject missing: %q", srv.received)
	}
}

func TestNotifications_PostTest_BadAddress(t *testing.T) {
	mgr, _ := smtpmgr.NewManager(smtpmgr.Config{}, 30)
	h := &NotificationsHandler{Logger: newDiscardLogger(), Mgr: mgr}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/smtp/test", strings.NewReader(`{"to":"nope"}`))
	rr := httptest.NewRecorder()
	h.PostTest(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestNotifications_PostTest_NotConfigured(t *testing.T) {
	mgr, _ := smtpmgr.NewManager(smtpmgr.Config{}, 30)
	h := &NotificationsHandler{Logger: newDiscardLogger(), Mgr: mgr}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/smtp/test", strings.NewReader(`{"to":"a@b.c"}`))
	rr := httptest.NewRecorder()
	h.PostTest(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}
