package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	var (
		addr    = flag.String("addr", getEnv("LADDER_ADDR", ":8080"), "address to listen on")
		proxyURL = flag.String("proxy", getEnv("LADDER_PROXY", ""), "upstream proxy URL")
		timeout = flag.Duration("timeout", 90*time.Second, "request timeout") // increased from 60s; some slow sites need even more time
		showVersion = flag.Bool("version", false, "print version and exit")
	)
	flag.Parse()

	if *showVersion {
		fmt.Printf("ladder version %s, commit %s, built %s\n", version, commit, date)
		os.Exit(0)
	}

	log.Printf("Starting ladder %s (commit: %s, built: %s)", version, commit, date)

	handler, err := NewHandler(&Config{
		ProxyURL: *proxyURL,
		Timeout:  *timeout,
	})
	if err != nil {
		log.Fatalf("failed to create handler: %v", err)
	}

	srv := &http.Server{
		Addr:         *addr,
		Handler:      handler,
		ReadTimeout:  *timeout,
		WriteTimeout: *timeout + 10*time.Second, // bumped extra buffer from 5s to 10s
		IdleTimeout:  120 * time.Second, // reduced from 240s; more reasonable for typical use
	}

	// Start server in a goroutine so we can listen for shutdown signals
	go func() {
		log.Printf("Listening on %s", *addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	// Wait for interrupt signal to gracefully shut down the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("server forced to shutdown: %v", err)
	}

	log.Println("Server exited")
}

// getEnv returns the value of an environment variable or a default value.
func getEnv(key, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return defaultVal
}
