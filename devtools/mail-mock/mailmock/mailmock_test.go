package mailmock

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/smtp"
	"strings"
	"testing"
)

func startSMTP(t *testing.T, store *Store) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = (&SMTPServer{Store: store}).Serve(ctx, lis) }()
	return lis.Addr().String()
}

// The same call metap's mailer and cron email target make: net/smtp.SendMail, no auth.
func TestNetSMTPDeliversAndTheAPIReadsItBack(t *testing.T) {
	store := NewStore(10)
	addr := startSMTP(t, store)
	msg := "From: Portal <no-reply@waf.local>\r\nTo: bob@acme.io\r\nSubject: =?UTF-8?B?" +
		"TOG7nWkgdGhhbSBnaWE=?=\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n" +
		"Content-Transfer-Encoding: quoted-printable\r\n\r\nOpen http://localhost:5173/accept-invite?token=3Dabc\r\n.leading dot\r\n"
	if err := smtp.SendMail(addr, nil, "no-reply@waf.local", []string{"bob@acme.io", "eve@acme.io"}, []byte(msg)); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(Handler(store))
	defer srv.Close()
	res, err := http.Get(srv.URL + "/api/messages/latest?to=BOB@acme.io")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var body struct{ Data Message }
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	m := body.Data
	if m.Subject != "Lời tham gia" {
		t.Errorf("subject = %q, want the decoded encoded-word", m.Subject)
	}
	if !strings.Contains(m.Text, "accept-invite?token=abc") {
		t.Errorf("text = %q, want quoted-printable decoded", m.Text)
	}
	if !strings.Contains(m.Text, "\n.leading dot") {
		t.Errorf("text = %q, want dot-stuffing undone", m.Text)
	}
	if len(m.To) != 2 || m.From != "no-reply@waf.local" {
		t.Errorf("envelope = %v / %q", m.To, m.From)
	}
	if got := store.List("nobody@acme.io"); len(got) != 0 {
		t.Errorf("filter by recipient: got %d", len(got))
	}
}

func TestAuthIsAcceptedAndCapacityDropsTheOldest(t *testing.T) {
	store := NewStore(2)
	addr := startSMTP(t, store)
	host, _, _ := net.SplitHostPort(addr)
	auth := smtp.PlainAuth("", "user", "pass", host)
	for i := 0; i < 3; i++ {
		msg := "Subject: n" + string(rune('0'+i)) + "\r\n\r\nbody\r\n"
		if err := smtp.SendMail(addr, auth, "a@x.io", []string{"b@x.io"}, []byte(msg)); err != nil {
			t.Fatal(err)
		}
	}
	got := store.List("")
	if len(got) != 2 || got[0].Subject != "n2" || got[1].Subject != "n1" {
		t.Fatalf("want the 2 newest, newest first, got %+v", got)
	}
	store.Clear()
	if len(store.List("")) != 0 {
		t.Fatal("clear left messages")
	}
}
