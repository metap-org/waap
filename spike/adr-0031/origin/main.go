// Origin stand-in for the spike: small static, large dynamic and slow responses.
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:9300", "listen")
	name := flag.String("name", "o1", "origin name returned by /id")
	flag.Parse()
	small := []byte(strings.Repeat("a", 4096))
	big := []byte(strings.Repeat("b", 25*1024))
	mux := http.NewServeMux()
	mux.HandleFunc("/static/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=300")
		w.Header().Set("Content-Type", "application/javascript")
		w.Write(small)
	})
	mux.HandleFunc("/page", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write(big)
	})
	mux.HandleFunc("/api/echo", func(w http.ResponseWriter, r *http.Request) {
		n, _ := io.Copy(io.Discard, r.Body)
		fmt.Fprintf(w, "{\"bytes\":%d}", n)
	})
	mux.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		ms, _ := strconv.Atoi(r.URL.Query().Get("ms"))
		time.Sleep(time.Duration(ms) * time.Millisecond)
		w.Write([]byte("ok"))
	})
	mux.HandleFunc("/id", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(*name)) })
	mux.HandleFunc("/cached", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=300")
		fmt.Fprintf(w, "%s %d", *name, time.Now().UnixNano())
	})
	// /stream?mb=N: chunked streaming response, N MiB.
	mux.HandleFunc("/stream", func(w http.ResponseWriter, r *http.Request) {
		mb, _ := strconv.Atoi(r.URL.Query().Get("mb"))
		chunk := make([]byte, 64*1024)
		fl, _ := w.(http.Flusher)
		for i := 0; i < mb*16; i++ {
			w.Write(chunk)
			if fl != nil && i%4 == 0 {
				fl.Flush()
			}
		}
	})
	// /ws: Upgrade tunnel that echoes raw bytes (enough to test that the proxy tunnels an upgraded connection).
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			http.Error(w, "no upgrade", 400)
			return
		}
		c, rw, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer c.Close()
		rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n")
		rw.Flush()
		io.Copy(c, rw)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("origin ok")) })
	log.Fatal((&http.Server{Addr: *addr, Handler: mux}).ListenAndServe())
}
