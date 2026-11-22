// Package mailmock is a catch-all SMTP server for dev and staging: it accepts every message,
// delivers none, and keeps them in memory for a UI and an API to read back (invitation and
// password-reset links in local runs, assertions in tests). Standard library only.
package mailmock

import (
	"bytes"
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Message is one received email.
type Message struct {
	ID         int       `json:"id"`
	ReceivedAt time.Time `json:"receivedAt"`
	// From/To are the SMTP envelope (MAIL FROM / RCPT TO), the addresses it was really sent to.
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	// Text is the plain-text body (the text/plain part of a multipart message), decoded.
	Text string `json:"text"`
	// HTML is the text/html part, when there is one.
	HTML string `json:"html,omitempty"`
	// Raw is the message exactly as received.
	Raw string `json:"raw"`
}

// Store keeps the newest messages, dropping the oldest past its capacity.
type Store struct {
	mu       sync.Mutex
	messages []Message
	nextID   int
	capacity int
	// boxes is per-mailbox client state (flags, deletions); see mailbox.go.
	boxes map[string]*boxState
}

func NewStore(capacity int) *Store {
	if capacity <= 0 {
		capacity = 500
	}
	return &Store{capacity: capacity, nextID: 1}
}

// Add parses raw and stores it; a message that doesn't parse is still kept with its raw text.
func (s *Store) Add(from string, to []string, raw []byte, now time.Time) Message {
	m := Message{From: from, To: to, Raw: string(raw), ReceivedAt: now}
	if parsed, err := mail.ReadMessage(bytes.NewReader(raw)); err == nil {
		dec := new(mime.WordDecoder)
		m.Subject = parsed.Header.Get("Subject")
		if decoded, err := dec.DecodeHeader(m.Subject); err == nil {
			m.Subject = decoded
		}
		m.Text, m.HTML = bodies(parsed.Header.Get("Content-Type"), parsed.Header.Get("Content-Transfer-Encoding"), parsed.Body)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	m.ID = s.nextID
	s.nextID++
	s.messages = append(s.messages, m)
	if len(s.messages) > s.capacity {
		s.messages = s.messages[len(s.messages)-s.capacity:]
	}
	return m
}

// List returns messages newest first; to, when set, keeps those sent to that address.
func (s *Store) List(to string) []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Message, 0, len(s.messages))
	for i := len(s.messages) - 1; i >= 0; i-- {
		if to == "" || hasRecipient(s.messages[i], to) {
			out = append(out, s.messages[i])
		}
	}
	return out
}

func (s *Store) Get(id int) (Message, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, m := range s.messages {
		if m.ID == id {
			return m, true
		}
	}
	return Message{}, false
}

func (s *Store) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = nil
}

func hasRecipient(m Message, addr string) bool {
	for _, to := range m.To {
		if strings.EqualFold(to, addr) {
			return true
		}
	}
	return false
}

// bodies walks a (possibly multipart) body and returns its text/plain and text/html parts.
func bodies(contentType, encoding string, body io.Reader) (text, html string) {
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = "text/plain"
	}
	if strings.HasPrefix(mediaType, "multipart/") {
		mr := multipart.NewReader(body, params["boundary"])
		for {
			part, err := mr.NextPart()
			if err != nil {
				break
			}
			t, h := bodies(part.Header.Get("Content-Type"), part.Header.Get("Content-Transfer-Encoding"), part)
			if text == "" {
				text = t
			}
			if html == "" {
				html = h
			}
		}
		return text, html
	}
	decoded := decode(encoding, body)
	if mediaType == "text/html" {
		return "", decoded
	}
	return decoded, ""
}

func decode(encoding string, body io.Reader) string {
	var r io.Reader = body
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "quoted-printable":
		r = quotedprintable.NewReader(body)
	case "base64":
		r = base64.NewDecoder(base64.StdEncoding, &stripNewlines{r: body})
	}
	b, _ := io.ReadAll(r)
	return string(b)
}

// stripNewlines feeds base64 bodies (wrapped at 76 columns) to a decoder that rejects line breaks.
type stripNewlines struct{ r io.Reader }

func (s *stripNewlines) Read(p []byte) (int, error) {
	n, err := s.r.Read(p)
	j := 0
	for _, c := range p[:n] {
		if c != '\r' && c != '\n' {
			p[j] = c
			j++
		}
	}
	return j, err
}

// parseID reads a message id from a URL path segment.
func parseID(s string) (int, bool) {
	n, err := strconv.Atoi(s)
	return n, err == nil && n > 0
}
