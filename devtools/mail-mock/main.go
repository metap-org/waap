// mail-mock — catch-all SMTP server for dev and staging (see package mailmock). It belongs to no
// plane: it stands in for the outside world's mail relay, so invitation and password-reset emails
// can be read without a real mailbox. It accepts everything and delivers nothing. Never use it as
// a production relay.
//
// Env:
//
//	MAIL_MOCK_SMTP_ADDR  SMTP listen address (default :1025)
//	MAIL_MOCK_HTTP_ADDR  UI (/) and API (/api/*) (default :8025)
//	MAIL_MOCK_IMAP_ADDR  IMAP listen address (default :1143, "off" to disable)
//	MAIL_MOCK_POP3_ADDR  POP3 listen address (default :1110, "off" to disable)
//	MAIL_MOCK_CAPACITY   messages kept in memory, oldest dropped first (default 500)
package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/metap-org/waap/devtools/mail-mock/mailmock"
)

func envOr(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

func main() {
	capacity, _ := strconv.Atoi(os.Getenv("MAIL_MOCK_CAPACITY"))
	store := mailmock.NewStore(capacity)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	smtpAddr := envOr("MAIL_MOCK_SMTP_ADDR", ":1025")
	lis, err := net.Listen("tcp", smtpAddr)
	if err != nil {
		slog.Error("listening for SMTP", "addr", smtpAddr, "error", err)
		os.Exit(1)
	}
	go func() {
		if err := (&mailmock.SMTPServer{Store: store}).Serve(ctx, lis); err != nil {
			slog.Error("SMTP server stopped", "error", err)
			stop()
		}
	}()

	// IMAP and POP3 read the same messages, one mailbox per recipient address (see mailbox.go).
	imapAddr, pop3Addr := envOr("MAIL_MOCK_IMAP_ADDR", ":1143"), envOr("MAIL_MOCK_POP3_ADDR", ":1110")
	if imapAddr != "off" {
		serveProtocol(ctx, stop, "IMAP", imapAddr, (&mailmock.IMAPServer{Store: store, UIDValidity: uint32(time.Now().Unix())}).Serve)
	} else {
		imapAddr = ""
	}
	if pop3Addr != "off" {
		serveProtocol(ctx, stop, "POP3", pop3Addr, (&mailmock.POP3Server{Store: store}).Serve)
	} else {
		pop3Addr = ""
	}

	httpAddr := envOr("MAIL_MOCK_HTTP_ADDR", ":8025")
	info := mailmock.Info{SMTP: smtpAddr, IMAP: imapAddr, POP3: pop3Addr}
	srv := &http.Server{Addr: httpAddr, Handler: mailmock.Handler(store, info), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	slog.Info("mail-mock listening", "smtp", smtpAddr, "imap", imapAddr, "pop3", pop3Addr, "http", httpAddr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("HTTP server stopped", "error", err)
		os.Exit(1)
	}
}

// serveProtocol listens on addr and runs serve in the background; a failure to listen is fatal, a
// server that stops later takes the process down with it.
func serveProtocol(ctx context.Context, stop context.CancelFunc, name, addr string, serve func(context.Context, net.Listener) error) {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		slog.Error("listening", "protocol", name, "addr", addr, "error", err)
		os.Exit(1)
	}
	go func() {
		if err := serve(ctx, lis); err != nil {
			slog.Error("server stopped", "protocol", name, "error", err)
			stop()
		}
	}()
}
