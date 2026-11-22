package mailmock

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

func seed(t *testing.T) *Store {
	t.Helper()
	s := NewStore(50)
	add := func(to, subject, body string) {
		raw := "From: Portal <no-reply@waf.local>\nTo: " + to + "\nSubject: " + subject + "\nDate: Sun, 22 Nov 2026 10:00:00 +0000\n\n" + body + "\n"
		s.Add("no-reply@waf.local", []string{to}, []byte(raw), time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC))
	}
	add("bob@acme.io", "Invite", "Join: http://x/accept?token=1\n.dot line")
	add("eve@acme.io", "Reset", "Reset: http://x/reset?token=2")
	add("bob@acme.io", "Second", "Hello again")
	return s
}

type client struct {
	t *testing.T
	c net.Conn
	r *bufio.Reader
}

func dial(t *testing.T, addr string) *client {
	t.Helper()
	c, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return &client{t: t, c: c, r: bufio.NewReader(c)}
}

func (c *client) line() string {
	c.t.Helper()
	_ = c.c.SetReadDeadline(time.Now().Add(3 * time.Second))
	l, err := c.r.ReadString('\n')
	if err != nil {
		c.t.Fatalf("read: %v", err)
	}
	return strings.TrimRight(l, "\r\n")
}

func (c *client) send(format string, a ...any) {
	c.t.Helper()
	fmt.Fprintf(c.c, format+"\r\n", a...)
}

// until reads lines up to and including the one for which stop is true.
func (c *client) until(stop func(string) bool) []string {
	c.t.Helper()
	var out []string
	for {
		l := c.line()
		out = append(out, l)
		if stop(l) {
			return out
		}
	}
}

func listen(t *testing.T, serve func(context.Context, net.Listener) error) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = serve(ctx, l) }()
	return l.Addr().String()
}

func TestPOP3ReadsDeletesAndHidesOnQuit(t *testing.T) {
	store := seed(t)
	addr := listen(t, (&POP3Server{Store: store}).Serve)
	c := dial(t, addr)
	if l := c.line(); !strings.HasPrefix(l, "+OK") {
		t.Fatal(l)
	}
	c.send("USER bob@acme.io")
	c.line()
	c.send("PASS anything")
	if l := c.line(); l != "+OK 2 messages" {
		t.Fatalf("pass: %q", l)
	}
	c.send("STAT")
	if l := c.line(); !strings.HasPrefix(l, "+OK 2 ") {
		t.Fatalf("stat: %q", l)
	}
	c.send("UIDL")
	lines := c.until(func(l string) bool { return l == "." })
	if len(lines) != 4 { // +OK, two entries, dot
		t.Fatalf("uidl: %v", lines)
	}
	c.send("RETR 1")
	got := c.until(func(l string) bool { return l == "." })
	joined := strings.Join(got, "\n")
	if !strings.Contains(joined, "Subject: Invite") || !strings.Contains(joined, "\n..dot line") {
		t.Errorf("retr should carry the message with dot-stuffing: %q", joined)
	}
	c.send("TOP 1 0")
	top := strings.Join(c.until(func(l string) bool { return l == "." }), "\n")
	if strings.Contains(top, "Join:") || !strings.Contains(top, "Subject: Invite") {
		t.Errorf("top 0 is the headers only: %q", top)
	}
	c.send("DELE 1")
	c.line()
	c.send("RETR 1")
	if l := c.line(); !strings.HasPrefix(l, "-ERR") {
		t.Errorf("deleted message should be gone: %q", l)
	}
	c.send("QUIT")
	c.line()

	// Hidden from bob's mailbox only; the store (UI, API) and eve's mailbox keep everything.
	if n := len(store.Mailbox("bob@acme.io")); n != 1 {
		t.Errorf("bob has %d, want 1", n)
	}
	if n := len(store.Mailbox("eve@acme.io")); n != 1 {
		t.Errorf("eve has %d, want 1", n)
	}
	if n := len(store.List("")); n != 3 {
		t.Errorf("store has %d, want 3", n)
	}
}

func imapLogin(t *testing.T, store *Store, user string) *client {
	t.Helper()
	addr := listen(t, (&IMAPServer{Store: store, UIDValidity: 7}).Serve)
	c := dial(t, addr)
	c.line() // greeting
	c.send("a1 LOGIN %s pw", user)
	if l := c.line(); !strings.HasPrefix(l, "a1 OK") {
		t.Fatal(l)
	}
	return c
}

func tagged(c *client, tag string) []string {
	return c.until(func(l string) bool { return strings.HasPrefix(l, tag+" ") })
}

func TestIMAPSelectFetchSearchStoreExpunge(t *testing.T) {
	store := seed(t)
	c := imapLogin(t, store, "bob@acme.io")

	c.send("a2 SELECT INBOX")
	sel := strings.Join(tagged(c, "a2"), "\n")
	for _, want := range []string{"* 2 EXISTS", "UIDVALIDITY 7", "UIDNEXT 4", "a2 OK [READ-WRITE]"} {
		if !strings.Contains(sel, want) {
			t.Errorf("select missing %q in:\n%s", want, sel)
		}
	}

	c.send("a3 SEARCH UNSEEN")
	if got := tagged(c, "a3")[0]; got != "* SEARCH 1 2" {
		t.Errorf("unseen = %q", got)
	}

	c.send("a4 FETCH 1 (FLAGS BODY[HEADER.FIELDS (Subject)] BODY[TEXT])")
	fetch := strings.Join(tagged(c, "a4"), "\n")
	if !strings.Contains(fetch, "Subject: Invite") || strings.Contains(fetch, "To: bob") {
		t.Errorf("header fields should carry only Subject: %q", fetch)
	}
	if !strings.Contains(fetch, "Join: http://x/accept?token=1") || !strings.Contains(fetch, `\Seen`) {
		t.Errorf("a non-peek BODY[TEXT] returns the text and marks it seen: %q", fetch)
	}

	c.send("a5 SEARCH UNSEEN")
	if got := tagged(c, "a5")[0]; got != "* SEARCH 2" {
		t.Errorf("after reading message 1, unseen = %q", got)
	}

	c.send("a6 UID FETCH 3 (UID RFC822.SIZE)")
	if got := tagged(c, "a6")[0]; !strings.HasPrefix(got, "* 2 FETCH (UID 3 RFC822.SIZE ") {
		t.Errorf("uid fetch = %q", got)
	}

	c.send(`a7 STORE 1 +FLAGS (\Deleted)`)
	if got := strings.Join(tagged(c, "a7"), "\n"); !strings.Contains(got, `\Deleted`) {
		t.Errorf("store should report the flags: %q", got)
	}
	c.send("a8 EXPUNGE")
	if got := tagged(c, "a8")[0]; got != "* 1 EXPUNGE" {
		t.Errorf("expunge = %q", got)
	}
	c.send("a9 STATUS INBOX (MESSAGES)")
	if got := tagged(c, "a9")[0]; !strings.Contains(got, "MESSAGES 1 ") {
		t.Errorf("status = %q", got)
	}
	c.send("a10 LOGOUT")
	tagged(c, "a10")
	if n := len(store.List("")); n != 3 {
		t.Errorf("the store keeps %d, want 3", n)
	}
}

func TestIMAPWildcardReadsEveryMailboxAndNewMailArrivesOnNoop(t *testing.T) {
	store := seed(t)
	c := imapLogin(t, store, "*")
	c.send("b1 SELECT INBOX")
	if sel := strings.Join(tagged(c, "b1"), "\n"); !strings.Contains(sel, "* 3 EXISTS") {
		t.Fatalf("the wildcard user reads everything: %s", sel)
	}
	store.Add("x@y", []string{"new@acme.io"}, []byte("Subject: Late\n\nbody\n"), time.Now())
	c.send("b2 NOOP")
	got := strings.Join(tagged(c, "b2"), "\n")
	if !strings.Contains(got, "* 4 EXISTS") {
		t.Errorf("a message that arrives while selected is announced on NOOP: %q", got)
	}
}

func TestIMAPRefusesBeforeLoginAndUnknownMailbox(t *testing.T) {
	store := seed(t)
	addr := listen(t, (&IMAPServer{Store: store}).Serve)
	c := dial(t, addr)
	c.line()
	c.send("t1 SELECT INBOX")
	if l := c.line(); !strings.HasPrefix(l, "t1 NO") {
		t.Errorf("select before login: %q", l)
	}
	c.send("t2 LOGIN bob@acme.io pw")
	c.line()
	c.send("t3 SELECT Drafts")
	if l := c.line(); !strings.HasPrefix(l, "t3 NO") {
		t.Errorf("only INBOX exists: %q", l)
	}
}
