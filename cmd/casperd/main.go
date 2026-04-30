// Package main implements casperd, the HTTP server backing the Casper
// dashboard. It is a thin layer over the same libraries casperctl uses —
// router, fetcher, proposer, policy, interpreter, audit. No business
// logic lives here.
//
// Defaults: bind 127.0.0.1:8787. Auth: none. Both decisions are
// deliberate for a single-operator alpha; tighten when this graduates
// off localhost.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/server"
)

func main() {
	// Load .env from the working directory if present. Existing env vars
	// take precedence — godotenv.Load only sets vars that aren't already set.
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		log.Printf("casperd: .env: %v", err)
	}

	addr := getenv("CASPERD_ADDR", "127.0.0.1:8787")

	srv := server.New(server.Options{
		Addr: addr,
		Deps: server.NewRuntimeDeps(),
	})

	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("casperd: listening on http://%s", addr)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("casperd: listen error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("casperd: shutdown requested")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		log.Printf("casperd: shutdown error: %v", err)
	}
	log.Println("casperd: bye")
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// silence unused-import warning if main grows
var _ = fmt.Sprintf
