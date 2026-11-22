package mailmock

import (
	"sort"
	"strings"
)

// Mailboxes: the same messages, read the way a mail client does (IMAP, POP3). There is one mailbox
// per recipient address, with no setup and no passwords: a client logs in as the address it wants to
// read (any password) and sees the messages sent to it. The user "*" reads every message. State a
// client sets (read, deleted, flagged) belongs to that mailbox alone: deleting a message in one
// recipient's mailbox neither removes it from another's nor from the web UI and the API.
//
// Messages a client deleted stay in the store (the UI is the audit view of everything the mock ever
// received) and are only hidden from that mailbox.

// WildcardUser is the mailbox user that reads every message.
const WildcardUser = "*"

// Entry is one message as one mailbox sees it.
type Entry struct {
	Message
	Flags []string
}

type boxState struct {
	flags map[int]map[string]bool
	gone  map[int]bool
}

func boxKey(user string) string { return strings.ToLower(strings.TrimSpace(user)) }

func (s *Store) box(user string) *boxState {
	if s.boxes == nil {
		s.boxes = map[string]*boxState{}
	}
	k := boxKey(user)
	b := s.boxes[k]
	if b == nil {
		b = &boxState{flags: map[int]map[string]bool{}, gone: map[int]bool{}}
		s.boxes[k] = b
	}
	return b
}

// Mailbox returns the messages one user sees, oldest first (the order IMAP numbers them in).
func (s *Store) Mailbox(user string) []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	b := s.box(user)
	all := boxKey(user) == WildcardUser
	var out []Entry
	for _, m := range s.messages {
		if b.gone[m.ID] || (!all && !hasRecipient(m, user)) {
			continue
		}
		out = append(out, Entry{Message: m, Flags: flagList(b.flags[m.ID])})
	}
	return out
}

func flagList(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for f, on := range set {
		if on {
			out = append(out, f)
		}
	}
	sort.Strings(out)
	return out
}

// SetFlag turns one flag on or off for a message in a mailbox.
func (s *Store) SetFlag(user string, id int, flag string, on bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b := s.box(user)
	if b.flags[id] == nil {
		b.flags[id] = map[string]bool{}
	}
	if on {
		b.flags[id][flag] = true
	} else {
		delete(b.flags[id], flag)
	}
}

// Hide removes messages from one mailbox's view (a client's EXPUNGE or POP3 DELE).
func (s *Store) Hide(user string, ids ...int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b := s.box(user)
	for _, id := range ids {
		b.gone[id] = true
	}
}

// NextID is the id the next message will get (IMAP UIDNEXT).
func (s *Store) NextID() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.nextID
}

// crlf gives a message the line endings mail protocols require (the SMTP reader hands over LF).
func crlf(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\n", "\r\n")
}

// splitMessage returns a message's header block (with its closing blank line) and its body.
func splitMessage(raw string) (header, body string) {
	raw = crlf(raw)
	if i := strings.Index(raw, "\r\n\r\n"); i >= 0 {
		return raw[:i+4], raw[i+4:]
	}
	return raw + "\r\n", ""
}
