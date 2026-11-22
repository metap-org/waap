package mailmock

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// POP3Server is a small RFC 1939 server over the mailboxes (mailbox.go): USER/PASS (any password),
// STAT, LIST, UIDL, RETR, TOP, DELE, NOOP, RSET, QUIT, plus CAPA. Messages marked with DELE are hidden
// from that mailbox when the client QUITs, as the RFC says. No TLS.
type POP3Server struct {
	Store    *Store
	Hostname string
}

// Serve accepts connections on l until ctx is done.
func (s *POP3Server) Serve(ctx context.Context, l net.Listener) error {
	return serveLoop(ctx, l, s.handle)
}

func (s *POP3Server) handle(conn net.Conn) {
	defer conn.Close()
	host := s.Hostname
	if host == "" {
		host = "mail-mock"
	}
	r := bufio.NewReader(conn)
	w := bufio.NewWriter(conn)
	line := func(format string, a ...any) {
		_ = conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
		fmt.Fprintf(w, format+"\r\n", a...)
		_ = w.Flush()
	}
	// multi writes a dot-terminated block, dot-stuffing lines that start with a dot.
	multi := func(text string) {
		_ = conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
		if text != "" {
			for _, l := range strings.Split(strings.TrimSuffix(text, "\r\n"), "\r\n") {
				if strings.HasPrefix(l, ".") {
					w.WriteString(".")
				}
				w.WriteString(l + "\r\n")
			}
		}
		w.WriteString(".\r\n")
		_ = w.Flush()
	}
	line("+OK %s POP3 mail-mock ready", host)

	var user string
	var box []Entry
	deleted := map[int]bool{} // 1-based message numbers marked by DELE
	authed := false
	pick := func(arg string) (int, bool) {
		n, err := strconv.Atoi(strings.TrimSpace(arg))
		if err != nil || n < 1 || n > len(box) || deleted[n] {
			line("-ERR no such message")
			return 0, false
		}
		return n, true
	}
	octets := func(e Entry) int { return len(crlf(e.Raw)) }

	for {
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Minute))
		text, err := r.ReadString('\n')
		if err != nil {
			return
		}
		verb, arg, _ := strings.Cut(strings.TrimSpace(text), " ")
		switch strings.ToUpper(verb) {
		case "CAPA":
			line("+OK capability list follows")
			multi("USER\r\nUIDL\r\nTOP\r\nRESP-CODES\r\nPIPELINING\r\nIMPLEMENTATION mail-mock")
		case "USER":
			if authed {
				line("-ERR already authenticated")
				continue
			}
			user = strings.TrimSpace(arg)
			line("+OK")
		case "PASS":
			if authed || user == "" {
				line("-ERR send USER first")
				continue
			}
			authed = true
			box = s.Store.Mailbox(user)
			line("+OK %d messages", len(box))
		case "QUIT":
			if authed {
				var ids []int
				for n := range deleted {
					ids = append(ids, box[n-1].ID)
				}
				s.Store.Hide(user, ids...)
			}
			line("+OK bye")
			return
		default:
			if !authed {
				line("-ERR not authenticated")
				continue
			}
			s.transaction(verb, arg, box, deleted, pick, octets, line, multi, user)
		}
	}
}

func (s *POP3Server) transaction(verb, arg string, box []Entry, deleted map[int]bool, pick func(string) (int, bool),
	octets func(Entry) int, line func(string, ...any), multi func(string), user string) {
	listing := func(each func(n int, e Entry) string) {
		if strings.TrimSpace(arg) != "" {
			if n, ok := pick(arg); ok {
				line("+OK %s", each(n, box[n-1]))
			}
			return
		}
		var b strings.Builder
		for i, e := range box {
			if !deleted[i+1] {
				b.WriteString(each(i+1, e) + "\r\n")
			}
		}
		line("+OK")
		if b.Len() == 0 {
			multi("")
			return
		}
		multi(b.String())
	}
	switch strings.ToUpper(verb) {
	case "STAT":
		count, size := 0, 0
		for i, e := range box {
			if !deleted[i+1] {
				count++
				size += octets(e)
			}
		}
		line("+OK %d %d", count, size)
	case "LIST":
		listing(func(n int, e Entry) string { return fmt.Sprintf("%d %d", n, octets(e)) })
	case "UIDL":
		listing(func(n int, e Entry) string { return fmt.Sprintf("%d mm%d", n, e.ID) })
	case "RETR":
		if n, ok := pick(arg); ok {
			e := box[n-1]
			s.Store.SetFlag(user, e.ID, `\Seen`, true)
			line("+OK %d octets", octets(e))
			multi(crlf(e.Raw))
		}
	case "TOP":
		numStr, linesStr, _ := strings.Cut(strings.TrimSpace(arg), " ")
		n, ok := pick(numStr)
		count, err := strconv.Atoi(strings.TrimSpace(linesStr))
		if !ok {
			return
		}
		if err != nil || count < 0 {
			line("-ERR syntax: TOP msg lines")
			return
		}
		head, body := splitMessage(box[n-1].Raw)
		bodyLines := strings.Split(strings.TrimSuffix(body, "\r\n"), "\r\n")
		if count < len(bodyLines) {
			bodyLines = bodyLines[:count]
		}
		line("+OK")
		multi(head + strings.Join(bodyLines, "\r\n"))
	case "DELE":
		if n, ok := pick(arg); ok {
			deleted[n] = true
			line("+OK message %d deleted", n)
		}
	case "RSET":
		for n := range deleted {
			delete(deleted, n)
		}
		line("+OK")
	case "NOOP":
		line("+OK")
	default:
		line("-ERR unknown command")
	}
}

// serveLoop accepts connections until ctx is done and runs handle for each.
func serveLoop(ctx context.Context, l net.Listener, handle func(net.Conn)) error {
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
			return err
		}
		go handle(conn)
	}
}
