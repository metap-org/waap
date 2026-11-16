package mailmock

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/textproto"
	"strings"
	"time"
)

// MaxMessageBytes bounds one DATA section.
const MaxMessageBytes = 10 << 20

// SMTPServer is a minimal RFC 5321 receiver: EHLO/HELO, MAIL, RCPT, DATA, RSET, NOOP, QUIT, plus
// AUTH (any credentials accepted, so a client configured for an authenticated relay works
// unchanged). No STARTTLS: clients that only try it opportunistically (Go's net/smtp) fall back to
// plaintext. It never relays anything anywhere.
type SMTPServer struct {
	Store    *Store
	Hostname string
	Now      func() time.Time
}

// Serve accepts connections on l until ctx is done.
func (s *SMTPServer) Serve(ctx context.Context, l net.Listener) error {
	go func() {
		<-ctx.Done()
		_ = l.Close()
	}()
	for {
		conn, err := l.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				continue
			}
			return err
		}
		go s.handle(conn)
	}
}

func (s *SMTPServer) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *SMTPServer) handle(conn net.Conn) {
	defer conn.Close()
	host := s.Hostname
	if host == "" {
		host = "mail-mock"
	}
	tp := textproto.NewConn(conn)
	reply := func(code int, msg string) bool {
		_ = conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
		return tp.PrintfLine("%d %s", code, msg) == nil
	}
	if !reply(220, host+" ESMTP mail-mock") {
		return
	}
	var from string
	var to []string
	reset := func() { from, to = "", nil }
	for {
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Minute))
		line, err := tp.ReadLine()
		if err != nil {
			return
		}
		verb, arg, _ := strings.Cut(line, " ")
		switch strings.ToUpper(verb) {
		case "EHLO":
			_ = tp.PrintfLine("250-%s", host)
			_ = tp.PrintfLine("250-SIZE %d", MaxMessageBytes)
			_ = tp.PrintfLine("250-8BITMIME")
			_ = tp.PrintfLine("250-AUTH PLAIN LOGIN")
			reply(250, "SMTPUTF8")
		case "HELO":
			reply(250, host)
		case "AUTH":
			if !s.auth(tp, arg) {
				return
			}
		case "MAIL":
			addr, ok := pathArg(arg, "FROM:")
			if !ok {
				reply(501, "syntax: MAIL FROM:<address>")
				continue
			}
			reset()
			from = addr
			reply(250, "OK")
		case "RCPT":
			addr, ok := pathArg(arg, "TO:")
			if !ok || addr == "" {
				reply(501, "syntax: RCPT TO:<address>")
				continue
			}
			to = append(to, addr)
			reply(250, "OK")
		case "DATA":
			if len(to) == 0 {
				reply(503, "need RCPT first")
				continue
			}
			reply(354, "end data with <CR><LF>.<CR><LF>")
			raw, err := readData(tp)
			if err != nil {
				reply(552, err.Error())
				return
			}
			m := s.Store.Add(from, to, raw, s.now())
			slog.Info("mail received", "id", m.ID, "from", from, "to", to, "subject", m.Subject)
			reply(250, fmt.Sprintf("OK queued as %d", m.ID))
			reset()
		case "RSET":
			reset()
			reply(250, "OK")
		case "NOOP":
			reply(250, "OK")
		case "QUIT":
			reply(221, "bye")
			return
		default:
			reply(502, "command not implemented")
		}
	}
}

// auth accepts any credentials: AUTH PLAIN [initial], AUTH LOGIN (two prompts).
func (s *SMTPServer) auth(tp *textproto.Conn, arg string) bool {
	mech, initial, _ := strings.Cut(arg, " ")
	switch strings.ToUpper(mech) {
	case "PLAIN":
		if initial == "" {
			if tp.PrintfLine("334 ") != nil {
				return false
			}
			if _, err := tp.ReadLine(); err != nil {
				return false
			}
		}
	case "LOGIN":
		for _, prompt := range []string{"VXNlcm5hbWU6", "UGFzc3dvcmQ6"} { // "Username:", "Password:"
			if tp.PrintfLine("334 %s", prompt) != nil {
				return false
			}
			if _, err := tp.ReadLine(); err != nil {
				return false
			}
		}
	default:
		return tp.PrintfLine("504 unsupported mechanism") == nil
	}
	return tp.PrintfLine("235 authenticated") == nil
}

// pathArg reads the address out of "FROM:<a@b> SIZE=1" / "TO:<a@b>".
func pathArg(arg, prefix string) (string, bool) {
	if len(arg) < len(prefix) || !strings.EqualFold(arg[:len(prefix)], prefix) {
		return "", false
	}
	rest := strings.TrimSpace(arg[len(prefix):])
	if !strings.HasPrefix(rest, "<") {
		addr, _, _ := strings.Cut(rest, " ")
		return addr, true
	}
	end := strings.Index(rest, ">")
	if end < 0 {
		return "", false
	}
	return rest[1:end], true
}

// readData reads the dot-terminated DATA section, undoing dot-stuffing.
func readData(tp *textproto.Conn) ([]byte, error) {
	r := bufio.NewReader(tp.DotReader())
	buf := make([]byte, 0, 4096)
	chunk := make([]byte, 32*1024)
	for {
		n, err := r.Read(chunk)
		buf = append(buf, chunk[:n]...)
		if len(buf) > MaxMessageBytes {
			return nil, errors.New("message too large")
		}
		if err != nil {
			return buf, nil
		}
	}
}
