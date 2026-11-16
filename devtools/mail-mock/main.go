// mail-mock — catch-all SMTP server for dev and staging (see package mailmock). It belongs to no
// plane: it stands in for the outside world's mail relay, so invitation and password-reset emails
// can be read without a real mailbox. It accepts everything and delivers nothing. Never use it as
// a production relay.
//
// Env:
//
//	MAIL_MOCK_SMTP_ADDR  SMTP listen address (default :1025)
//	MAIL_MOCK_HTTP_ADDR  UI (/) and API (/api/*) (default :8025)
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

	httpAddr := envOr("MAIL_MOCK_HTTP_ADDR", ":8025")
	srv := &http.Server{Addr: httpAddr, Handler: mailmock.Handler(store), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	slog.Info("mail-mock listening", "smtp", smtpAddr, "http", httpAddr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("HTTP server stopped", "error", err)
		os.Exit(1)
	}
}
