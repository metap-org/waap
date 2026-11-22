package mailmock

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"net/mail"
	"strconv"
	"strings"
	"time"
)

// IMAPServer is a small IMAP4rev1 server over the mailboxes (mailbox.go): enough for real clients and
// for tests that read mail the way an application would. One folder, INBOX. Supported: CAPABILITY,
// NOOP, LOGOUT, LOGIN and AUTHENTICATE PLAIN (any password), NAMESPACE, LIST/LSUB, STATUS, SELECT and
// EXAMINE, CHECK, CLOSE, UNSELECT, EXPUNGE, FETCH, SEARCH, STORE and their UID forms, IDLE.
// Not supported: folders other than INBOX, APPEND, COPY, TLS, BODYSTRUCTURE detail (a plain-text
// stand-in is given).
type IMAPServer struct {
	Store    *Store
	Hostname string
	// UIDValidity is fixed for the life of the process, so UIDs (message ids) mean the same thing to a
	// client between sessions.
	UIDValidity uint32
}

// Serve accepts connections on l until ctx is done.
func (s *IMAPServer) Serve(ctx context.Context, l net.Listener) error {
	return serveLoop(ctx, l, s.handle)
}

type imapSession struct {
	s        *IMAPServer
	conn     net.Conn
	r        *bufio.Reader
	w        *bufio.Writer
	user     string
	selected bool
	readOnly bool
	box      []Entry // the selected mailbox, oldest first; index+1 is the sequence number
}

func (s *IMAPServer) handle(conn net.Conn) {
	defer conn.Close()
	host := s.Hostname
	if host == "" {
		host = "mail-mock"
	}
	sess := &imapSession{s: s, conn: conn, r: bufio.NewReader(conn), w: bufio.NewWriter(conn)}
	sess.out("* OK [CAPABILITY IMAP4rev1 AUTH=PLAIN IDLE UIDPLUS UNSELECT] %s IMAP mail-mock ready", host)
	for {
		_ = conn.SetReadDeadline(time.Now().Add(30 * time.Minute))
		text, err := sess.r.ReadString('\n')
		if err != nil {
			return
		}
		tag, rest, _ := strings.Cut(strings.TrimRight(text, "\r\n"), " ")
		cmd, args, _ := strings.Cut(rest, " ")
		if tag == "" || cmd == "" {
			sess.out("* BAD empty command")
			continue
		}
		if !sess.dispatch(tag, strings.ToUpper(cmd), strings.TrimSpace(args)) {
			return
		}
	}
}

func (x *imapSession) out(format string, a ...any) {
	_ = x.conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
	fmt.Fprintf(x.w, format+"\r\n", a...)
	_ = x.w.Flush()
}

func (x *imapSession) done(tag, status, text string) { x.out("%s %s %s", tag, status, text) }

// dispatch runs one command and reports whether the connection stays open.
func (x *imapSession) dispatch(tag, cmd, args string) bool {
	switch cmd {
	case "CAPABILITY":
		x.out("* CAPABILITY IMAP4rev1 AUTH=PLAIN IDLE UIDPLUS UNSELECT NAMESPACE")
		x.done(tag, "OK", "CAPABILITY completed")
	case "NOOP":
		x.refresh()
		x.done(tag, "OK", "NOOP completed")
	case "LOGOUT":
		x.out("* BYE mail-mock closing")
		x.done(tag, "OK", "LOGOUT completed")
		return false
	case "LOGIN":
		toks := tokens(args)
		if len(toks) < 2 {
			x.done(tag, "BAD", "LOGIN needs a user and a password")
			return true
		}
		x.user = unquote(toks[0])
		x.done(tag, "OK", "LOGIN completed")
	case "AUTHENTICATE":
		mech, initial, _ := strings.Cut(args, " ")
		if !strings.EqualFold(mech, "PLAIN") {
			x.done(tag, "NO", "only PLAIN is supported")
			return true
		}
		if initial == "" {
			x.out("+ ")
			line, err := x.r.ReadString('\n')
			if err != nil {
				return false
			}
			initial = strings.TrimSpace(line)
		}
		raw, err := base64.StdEncoding.DecodeString(initial)
		parts := strings.Split(string(raw), "\x00")
		if err != nil || len(parts) < 3 {
			x.done(tag, "BAD", "malformed PLAIN response")
			return true
		}
		x.user = parts[1]
		if x.user == "" {
			x.user = parts[0]
		}
		x.done(tag, "OK", "AUTHENTICATE completed")
	default:
		if x.user == "" {
			x.done(tag, "NO", "log in first")
			return true
		}
		x.authenticated(tag, cmd, args)
	}
	return true
}

func (x *imapSession) authenticated(tag, cmd, args string) {
	switch cmd {
	case "NAMESPACE":
		x.out(`* NAMESPACE (("" "/")) NIL NIL`)
		x.done(tag, "OK", "NAMESPACE completed")
	case "LIST", "LSUB":
		toks := tokens(args)
		if len(toks) >= 2 && unquote(toks[1]) == "" {
			x.out(`* %s (\Noselect) "/" ""`, cmd)
		} else {
			x.out(`* %s (\HasNoChildren) "/" "INBOX"`, cmd)
		}
		x.done(tag, "OK", cmd+" completed")
	case "SUBSCRIBE", "UNSUBSCRIBE":
		x.done(tag, "OK", cmd+" completed")
	case "CREATE", "DELETE", "RENAME", "APPEND", "COPY":
		x.done(tag, "NO", "mail-mock has only INBOX and is read-only apart from flags")
	case "STATUS":
		box := x.s.Store.Mailbox(x.user)
		unseen := 0
		for _, e := range box {
			if !hasFlag(e, `\Seen`) {
				unseen++
			}
		}
		x.out(`* STATUS "INBOX" (MESSAGES %d UIDNEXT %d UIDVALIDITY %d UNSEEN %d)`, len(box), x.s.Store.NextID(), x.s.UIDValidity, unseen)
		x.done(tag, "OK", "STATUS completed")
	case "SELECT", "EXAMINE":
		toks := tokens(args)
		if len(toks) == 0 || !strings.EqualFold(unquote(toks[0]), "INBOX") {
			x.done(tag, "NO", "no such mailbox (only INBOX)")
			return
		}
		x.box, x.selected, x.readOnly = x.s.Store.Mailbox(x.user), true, cmd == "EXAMINE"
		x.out(`* FLAGS (\Seen \Answered \Flagged \Deleted \Draft)`)
		x.out(`* OK [PERMANENTFLAGS (\Seen \Answered \Flagged \Deleted \Draft \*)]`)
		x.out("* %d EXISTS", len(x.box))
		x.out("* 0 RECENT")
		x.out("* OK [UIDVALIDITY %d]", x.s.UIDValidity)
		x.out("* OK [UIDNEXT %d]", x.s.Store.NextID())
		mode := "READ-WRITE"
		if x.readOnly {
			mode = "READ-ONLY"
		}
		x.done(tag, "OK", "["+mode+"] "+cmd+" completed")
	case "CHECK":
		x.refresh()
		x.done(tag, "OK", "CHECK completed")
	case "UNSELECT":
		x.selected, x.box = false, nil
		x.done(tag, "OK", "UNSELECT completed")
	case "CLOSE":
		if x.selected && !x.readOnly {
			x.expunge(false)
		}
		x.selected, x.box = false, nil
		x.done(tag, "OK", "CLOSE completed")
	case "EXPUNGE":
		if !x.need(tag, true) {
			return
		}
		x.expunge(true)
		x.done(tag, "OK", "EXPUNGE completed")
	case "FETCH", "SEARCH", "STORE":
		if !x.need(tag, false) {
			return
		}
		x.messages(tag, cmd, args, false)
	case "UID":
		sub, rest, _ := strings.Cut(args, " ")
		switch strings.ToUpper(sub) {
		case "FETCH", "SEARCH", "STORE":
			if !x.need(tag, false) {
				return
			}
			x.messages(tag, strings.ToUpper(sub), rest, true)
		default:
			x.done(tag, "BAD", "unsupported UID command")
		}
	case "IDLE":
		if !x.need(tag, false) {
			return
		}
		x.idle(tag)
	default:
		x.done(tag, "BAD", "unknown command "+cmd)
	}
}

func (x *imapSession) need(tag string, write bool) bool {
	if !x.selected {
		x.done(tag, "NO", "select a mailbox first")
		return false
	}
	if write && x.readOnly {
		x.done(tag, "NO", "mailbox is read-only")
		return false
	}
	return true
}

// refresh picks up messages that arrived since the last look and announces them.
func (x *imapSession) refresh() {
	if !x.selected {
		return
	}
	fresh := x.s.Store.Mailbox(x.user)
	have := map[int]bool{}
	for _, e := range x.box {
		have[e.ID] = true
	}
	grew := false
	for _, e := range fresh {
		if !have[e.ID] {
			x.box = append(x.box, e)
			grew = true
		}
	}
	if grew {
		x.out("* %d EXISTS", len(x.box))
		x.out("* 0 RECENT")
	}
}

func (x *imapSession) idle(tag string) {
	x.out("+ idling")
	stop := make(chan struct{})
	go func() {
		defer close(stop)
		for {
			line, err := x.r.ReadString('\n')
			if err != nil || strings.EqualFold(strings.TrimSpace(line), "DONE") {
				return
			}
		}
	}()
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-stop:
			x.done(tag, "OK", "IDLE terminated")
			return
		case <-tick.C:
			x.refresh()
		}
	}
}

// expunge removes \Deleted messages from the mailbox; announce sends the untagged EXPUNGE lines.
func (x *imapSession) expunge(announce bool) {
	var ids []int
	kept := x.box[:0:0]
	seq := 0
	for _, e := range x.box {
		seq++
		if hasFlag(e, `\Deleted`) {
			ids = append(ids, e.ID)
			if announce {
				// Each removal renumbers the ones after it, so the number to report is the position
				// among those still there.
				x.out("* %d EXPUNGE", len(kept)+1)
			}
			continue
		}
		kept = append(kept, e)
	}
	x.s.Store.Hide(x.user, ids...)
	x.box = kept
}

func hasFlag(e Entry, flag string) bool {
	for _, f := range e.Flags {
		if strings.EqualFold(f, flag) {
			return true
		}
	}
	return false
}

// messages runs FETCH, SEARCH or STORE, by sequence number or (uid) by message id.
func (x *imapSession) messages(tag, cmd, args string, uid bool) {
	label := cmd
	if uid {
		label = "UID " + cmd
	}
	switch cmd {
	case "SEARCH":
		matches := x.search(args, uid)
		var b strings.Builder
		b.WriteString("* SEARCH")
		for _, n := range matches {
			b.WriteString(" " + strconv.Itoa(n))
		}
		x.out("%s", b.String())
		x.done(tag, "OK", label+" completed")
		return
	}
	setStr, rest, _ := strings.Cut(args, " ")
	idx := x.resolve(setStr, uid)
	rest = strings.TrimSpace(rest)
	switch cmd {
	case "FETCH":
		items := fetchItems(rest)
		if uid && !hasItem(items, "UID") {
			items = append([]string{"UID"}, items...)
		}
		for _, i := range idx {
			x.fetchOne(i, items)
		}
	case "STORE":
		if x.readOnly {
			x.done(tag, "NO", "mailbox is read-only")
			return
		}
		x.store(idx, rest, uid)
	}
	x.done(tag, "OK", label+" completed")
}

// resolve turns a sequence/UID set ("1:3,5", "2:*") into indexes into x.box.
func (x *imapSession) resolve(set string, uid bool) []int {
	var out []int
	seen := map[int]bool{}
	key := func(i int) int {
		if uid {
			return x.box[i].ID
		}
		return i + 1
	}
	maxKey := 0
	for i := range x.box {
		if k := key(i); k > maxKey {
			maxKey = k
		}
	}
	num := func(s string) int {
		if s == "*" {
			return maxKey
		}
		n, _ := strconv.Atoi(s)
		return n
	}
	for _, part := range strings.Split(set, ",") {
		lo, hi, isRange := strings.Cut(part, ":")
		a := num(lo)
		b := a
		if isRange {
			b = num(hi)
		}
		if a > b {
			a, b = b, a
		}
		for i := range x.box {
			if k := key(i); k >= a && k <= b && !seen[i] {
				seen[i] = true
				out = append(out, i)
			}
		}
	}
	return out
}

func (x *imapSession) store(idx []int, rest string, uid bool) {
	mode, flagsStr, _ := strings.Cut(rest, " ")
	silent := strings.HasSuffix(strings.ToUpper(mode), ".SILENT")
	op := strings.TrimSuffix(strings.ToUpper(mode), ".SILENT")
	var flags []string
	for _, f := range strings.Fields(strings.Trim(strings.TrimSpace(flagsStr), "()")) {
		flags = append(flags, f)
	}
	for _, i := range idx {
		e := &x.box[i]
		if op == "FLAGS" {
			for _, old := range e.Flags {
				x.s.Store.SetFlag(x.user, e.ID, old, false)
			}
		}
		for _, f := range flags {
			x.s.Store.SetFlag(x.user, e.ID, f, op != "-FLAGS")
		}
		e.Flags = flagList2(x.s.Store.flagsOf(x.user, e.ID))
		if !silent {
			x.out("* %d FETCH (%sFLAGS (%s))", i+1, uidPrefix(uid, e.ID), strings.Join(e.Flags, " "))
		}
	}
}

func uidPrefix(uid bool, id int) string {
	if uid {
		return fmt.Sprintf("UID %d ", id)
	}
	return ""
}

func flagList2(set map[string]bool) []string { return flagList(set) }

func (s *Store) flagsOf(user string, id int) map[string]bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]bool{}
	for f, on := range s.box(user).flags[id] {
		out[f] = on
	}
	return out
}

// search supports ALL, SEEN, UNSEEN, DELETED, UNDELETED, FLAGGED, UNFLAGGED, FROM, TO, SUBJECT, TEXT,
// BODY, UID <set>, and a bare sequence set, ANDed together. It returns sequence numbers, or UIDs
// for UID SEARCH.
func (x *imapSession) search(args string, uid bool) []int {
	toks := tokens(args)
	var conds []func(i int) bool
	for k := 0; k < len(toks); k++ {
		t := strings.ToUpper(unquote(toks[k]))
		arg := func() string {
			k++
			if k < len(toks) {
				return unquote(toks[k])
			}
			return ""
		}
		contains := func(hay, needle string) bool {
			return strings.Contains(strings.ToLower(hay), strings.ToLower(needle))
		}
		switch t {
		case "ALL":
		case "SEEN":
			conds = append(conds, func(i int) bool { return hasFlag(x.box[i], `\Seen`) })
		case "UNSEEN":
			conds = append(conds, func(i int) bool { return !hasFlag(x.box[i], `\Seen`) })
		case "DELETED":
			conds = append(conds, func(i int) bool { return hasFlag(x.box[i], `\Deleted`) })
		case "UNDELETED":
			conds = append(conds, func(i int) bool { return !hasFlag(x.box[i], `\Deleted`) })
		case "FLAGGED":
			conds = append(conds, func(i int) bool { return hasFlag(x.box[i], `\Flagged`) })
		case "UNFLAGGED":
			conds = append(conds, func(i int) bool { return !hasFlag(x.box[i], `\Flagged`) })
		case "FROM":
			v := arg()
			conds = append(conds, func(i int) bool { return contains(x.box[i].From, v) })
		case "TO":
			v := arg()
			conds = append(conds, func(i int) bool { return contains(strings.Join(x.box[i].To, ","), v) })
		case "SUBJECT":
			v := arg()
			conds = append(conds, func(i int) bool { return contains(x.box[i].Subject, v) })
		case "TEXT", "BODY":
			v := arg()
			conds = append(conds, func(i int) bool { return contains(x.box[i].Text+x.box[i].HTML, v) })
		case "UID":
			set := map[int]bool{}
			for _, i := range x.resolve(arg(), true) {
				set[i] = true
			}
			conds = append(conds, func(i int) bool { return set[i] })
		default:
			if t != "" && (t[0] >= '0' && t[0] <= '9' || t[0] == '*') {
				set := map[int]bool{}
				for _, i := range x.resolve(t, false) {
					set[i] = true
				}
				conds = append(conds, func(i int) bool { return set[i] })
			}
		}
	}
	var out []int
	for i := range x.box {
		ok := true
		for _, c := range conds {
			ok = ok && c(i)
		}
		if ok {
			if uid {
				out = append(out, x.box[i].ID)
			} else {
				out = append(out, i+1)
			}
		}
	}
	return out
}

// fetchItems expands the macros and splits "(FLAGS UID BODY[HEADER.FIELDS (From To)])" into items.
func hasItem(items []string, name string) bool {
	for _, i := range items {
		if strings.EqualFold(i, name) {
			return true
		}
	}
	return false
}

func fetchItems(rest string) []string {
	rest = strings.TrimSpace(rest)
	switch strings.ToUpper(rest) {
	case "ALL":
		return []string{"FLAGS", "INTERNALDATE", "RFC822.SIZE", "ENVELOPE"}
	case "FAST":
		return []string{"FLAGS", "INTERNALDATE", "RFC822.SIZE"}
	case "FULL":
		return []string{"FLAGS", "INTERNALDATE", "RFC822.SIZE", "ENVELOPE", "BODY"}
	}
	if strings.HasPrefix(rest, "(") && strings.HasSuffix(rest, ")") {
		rest = rest[1 : len(rest)-1]
	}
	return tokens(rest)
}

func (x *imapSession) fetchOne(i int, items []string) {
	e := x.box[i]
	head, body := splitMessage(e.Raw)
	full := head + body
	var parts []string
	markSeen := false
	for _, item := range items {
		up := strings.ToUpper(item)
		switch {
		case up == "UID":
			parts = append(parts, fmt.Sprintf("UID %d", e.ID))
		case up == "FLAGS":
			parts = append(parts, fmt.Sprintf("FLAGS (%s)", strings.Join(e.Flags, " ")))
		case up == "RFC822.SIZE":
			parts = append(parts, fmt.Sprintf("RFC822.SIZE %d", len(full)))
		case up == "INTERNALDATE":
			parts = append(parts, fmt.Sprintf(`INTERNALDATE "%s"`, e.ReceivedAt.Format("02-Jan-2006 15:04:05 -0700")))
		case up == "ENVELOPE":
			parts = append(parts, "ENVELOPE "+envelope(e, head))
		case up == "BODY" || up == "BODYSTRUCTURE":
			parts = append(parts, fmt.Sprintf(`%s ("TEXT" "PLAIN" ("CHARSET" "UTF-8") NIL NIL "8BIT" %d %d)`, up, len(body), strings.Count(body, "\r\n")))
		case up == "RFC822":
			markSeen = true
			parts = append(parts, literal("RFC822", full))
		case up == "RFC822.HEADER":
			parts = append(parts, literal("RFC822.HEADER", head))
		case up == "RFC822.TEXT":
			markSeen = true
			parts = append(parts, literal("RFC822.TEXT", body))
		case strings.HasPrefix(up, "BODY[") || strings.HasPrefix(up, "BODY.PEEK["):
			peek := strings.HasPrefix(up, "BODY.PEEK[")
			if !peek {
				markSeen = true
			}
			section, partial := sectionOf(item)
			data := sectionData(section, head, body)
			label := "BODY[" + section + "]"
			if partial != "" {
				data, label = applyPartial(data, partial), label+"<"+strings.Split(partial, ".")[0]+">"
			}
			parts = append(parts, literal(label, data))
		}
	}
	if markSeen && !hasFlag(e, `\Seen`) && !x.readOnly {
		x.s.Store.SetFlag(x.user, e.ID, `\Seen`, true)
		x.box[i].Flags = flagList(x.s.Store.flagsOf(x.user, e.ID))
		parts = append(parts, fmt.Sprintf("FLAGS (%s)", strings.Join(x.box[i].Flags, " ")))
	}
	_ = x.conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
	fmt.Fprintf(x.w, "* %d FETCH (%s)\r\n", i+1, strings.Join(parts, " "))
	_ = x.w.Flush()
}

func literal(name, data string) string { return fmt.Sprintf("%s {%d}\r\n%s", name, len(data), data) }

// sectionOf reads "BODY.PEEK[HEADER.FIELDS (From To)]<0.100>" into its section and partial spec.
func sectionOf(item string) (section, partial string) {
	open := strings.Index(item, "[")
	end := strings.LastIndex(item, "]")
	if open < 0 || end < open {
		return "", ""
	}
	section = strings.TrimSpace(item[open+1 : end])
	if tail := item[end+1:]; strings.HasPrefix(tail, "<") && strings.HasSuffix(tail, ">") {
		partial = tail[1 : len(tail)-1]
	}
	return section, partial
}

func sectionData(section, head, body string) string {
	up := strings.ToUpper(section)
	switch {
	case up == "":
		return head + body
	case up == "HEADER":
		return head
	case up == "TEXT" || up == "1":
		return body
	case strings.HasPrefix(up, "HEADER.FIELDS"):
		open, end := strings.Index(section, "("), strings.LastIndex(section, ")")
		if open < 0 || end < open {
			return head
		}
		want := map[string]bool{}
		for _, f := range strings.Fields(section[open+1 : end]) {
			want[strings.ToLower(f)] = true
		}
		var b strings.Builder
		keep := false
		for _, l := range strings.SplitAfter(strings.TrimSuffix(head, "\r\n"), "\r\n") {
			if l == "" {
				continue
			}
			if l[0] != ' ' && l[0] != '\t' { // a header line starts a field; continuations follow it
				name, _, _ := strings.Cut(l, ":")
				keep = want[strings.ToLower(strings.TrimSpace(name))]
			}
			if keep {
				b.WriteString(l)
			}
		}
		b.WriteString("\r\n")
		return b.String()
	}
	return ""
}

func applyPartial(data, spec string) string {
	from, count, _ := strings.Cut(spec, ".")
	start, _ := strconv.Atoi(from)
	if start > len(data) {
		return ""
	}
	data = data[start:]
	if n, err := strconv.Atoi(count); err == nil && n < len(data) {
		data = data[:n]
	}
	return data
}

func envelope(e Entry, head string) string {
	msg, err := mail.ReadMessage(strings.NewReader(head + "\r\n"))
	nstr := func(s string) string {
		if s == "" {
			return "NIL"
		}
		return quote(s)
	}
	addrs := func(list []*mail.Address) string {
		if len(list) == 0 {
			return "NIL"
		}
		var b strings.Builder
		b.WriteString("(")
		for _, a := range list {
			local, host, _ := strings.Cut(a.Address, "@")
			fmt.Fprintf(&b, "(%s NIL %s %s)", nstr(a.Name), nstr(local), nstr(host))
		}
		b.WriteString(")")
		return b.String()
	}
	if err != nil {
		return "(NIL " + nstr(e.Subject) + " NIL NIL NIL NIL NIL NIL NIL NIL)"
	}
	from, _ := msg.Header.AddressList("From")
	to, _ := msg.Header.AddressList("To")
	cc, _ := msg.Header.AddressList("Cc")
	return fmt.Sprintf("(%s %s %s %s %s %s %s NIL NIL %s)",
		nstr(msg.Header.Get("Date")), nstr(e.Subject), addrs(from), addrs(from), addrs(from), addrs(to), addrs(cc), nstr(msg.Header.Get("Message-Id")))
}

func quote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\r", " ", "\n", " ").Replace(s) + `"`
}

// tokens splits an IMAP argument string on spaces, keeping quoted strings, (parenthesised lists)
// and [bracketed sections] whole.
func tokens(s string) []string {
	var out []string
	var cur strings.Builder
	depth, quoted := 0, false
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
		}
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quoted:
			cur.WriteByte(c)
			if c == '\\' && i+1 < len(s) {
				i++
				cur.WriteByte(s[i])
			} else if c == '"' {
				quoted = false
			}
		case c == '"':
			quoted = true
			cur.WriteByte(c)
		case c == '(' || c == '[':
			depth++
			cur.WriteByte(c)
		case c == ')' || c == ']':
			if depth > 0 {
				depth--
			}
			cur.WriteByte(c)
		case c == ' ' && depth == 0:
			flush()
		default:
			cur.WriteByte(c)
		}
	}
	flush()
	return out
}

func unquote(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return strings.NewReplacer(`\"`, `"`, `\\`, `\`).Replace(s[1 : len(s)-1])
	}
	return s
}
