// STAY — Shells That Await You. A persistent web terminal daemon; see docs/SPEC.md.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/bbushvt/stay/internal/config"
	"github.com/bbushvt/stay/internal/server"
	"github.com/bbushvt/stay/internal/terminal"
	"github.com/bbushvt/stay/web"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:7681", "loopback address to listen on")
	configPath := flag.String("config", "", "layout file (default ~/.config/stay/layout.yaml, else built-in layout)")
	flag.Parse()

	if err := requireLoopback(*listen); err != nil {
		log.Fatal(err)
	}

	cfg := loadConfig(*configPath)
	defs, err := cfg.Defs(os.Getenv)
	if err != nil {
		log.Fatal(err)
	}
	terms, err := terminal.NewManager(defs)
	if err != nil {
		log.Fatal(err)
	}
	defer terms.Close()

	srv := &http.Server{
		Addr:              *listen,
		Handler:           server.New(terms, cfg.Layout(os.Getenv), web.Assets()).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		_ = srv.Close() // websockets are hijacked; Shutdown doesn't wait for them
	}()

	log.Printf("stay listening on http://%s", *listen)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

// requireLoopback refuses to serve an unauthenticated shell on a non-loopback address.
func requireLoopback(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid --listen %q: %w", addr, err)
	}
	if host == "localhost" {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return nil
	}
	return fmt.Errorf("--listen %q is not a loopback address; STAY has no authentication of its own", addr)
}

// loadConfig reads the explicit --config file, else the default location, else
// falls back to the built-in layout. A file that exists but is invalid is fatal.
func loadConfig(explicit string) *config.Config {
	path := explicit
	if path == "" {
		path = config.DefaultPath()
	}
	if path != "" {
		cfg, err := config.Load(path)
		switch {
		case err == nil:
			log.Printf("layout loaded from %s", path)
			for _, id := range cfg.Unplaced() {
				log.Printf("warning: terminal %q is not placed in any tab and will be invisible", id)
			}
			return cfg
		case explicit == "" && errors.Is(err, fs.ErrNotExist):
			// no user config: use the built-in layout
		default:
			log.Fatal(err)
		}
	}
	_, lookErr := exec.LookPath("claude")
	log.Printf("no layout file; using built-in layout")
	return config.Default(lookErr == nil)
}
