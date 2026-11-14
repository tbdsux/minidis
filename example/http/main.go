// Command http receives interactions over HTTP instead of the gateway. Set the
// app's Interactions Endpoint URL in the Developer Portal to this server, which
// must be reachable over HTTPS, for example through a tunnel or reverse proxy.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/tbdsux/minidis"
)

func main() {
	guilds := strings.Split(os.Getenv("GUILD"), ",")
	bot := minidis.New(os.Getenv("TOKEN"))

	bot.AddCommand(&minidis.Command{
		Name:        "ping",
		Description: "Simple ping command.",
		Execute: func(c *minidis.Context) error {
			return c.Reply("pong, over HTTP")
		},
	})

	if err := bot.SyncCommands(guilds...); err != nil {
		log.Fatalln("Failed to sync commands:", err)
	}

	// PUBLIC_KEY is on the General Information page of the Developer Portal.
	handler, err := bot.HTTPHandler(os.Getenv("PUBLIC_KEY"))
	if err != nil {
		log.Fatalln(err)
	}

	mux := http.NewServeMux()
	mux.Handle("POST /interactions", handler)

	srv := &http.Server{
		Addr:              ":8080",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()
	log.Println("Listening on :8080/interactions")

	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Fatalln(err)
		}
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Stop taking requests, then wait for the handlers that are still running.
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Println("HTTP shutdown:", err)
	}
	if err := bot.Shutdown(shutdownCtx); err != nil {
		log.Println("Bot shutdown:", err)
	}
}
