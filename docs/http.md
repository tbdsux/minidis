# Receiving interactions over HTTP

Instead of the gateway, Discord can send interactions to the app's **Interactions Endpoint URL**, as HTTP requests. Such a bot needs no long-lived connection, so it suits serverless hosting and scaling behind a load balancer. It gets no gateway events, so `OnMessage`, `minidis.On`, text commands and the [state cache](./state.md) do not work.

Discord sends interactions one way or the other, so a bot uses either `Open`/`Run` or HTTP, not both.

```go
bot := interactions.New(os.Getenv("TOKEN"))
bot.AddCommand(&interactions.Command{
	Name:        "ping",
	Description: "Replies with pong.",
	Execute: func(c *interactions.Context) error {
		return c.Reply("pong, over HTTP")
	},
})

// Create the handler first, so that a bad key fails before syncing
handler, err := bot.HTTPHandler(ctx, os.Getenv("PUBLIC_KEY"))
if err != nil {
	log.Fatal(err)
}
if err := bot.SyncCommands(ctx); err != nil {
	log.Fatal(err)
}

mux := http.NewServeMux()
mux.Handle("POST /interactions", handler)

srv := &http.Server{
	Addr:              ":8080",
	Handler:           mux,
	ReadHeaderTimeout: 5 * time.Second,
	WriteTimeout:      20 * time.Second, // a slow response may take up to 15s to reach Discord
}
go srv.ListenAndServe()

<-ctx.Done()
srv.Shutdown(shutdownCtx) // stop taking requests...
bot.Shutdown(shutdownCtx) // ...then wait for running handlers
```

The [http example](../example/http/main.go) is the complete program. `minidis.Bot` has `HTTPHandler` too, but registers commands and connects to the gateway in `Run`, so serve the handler without calling `Run` or `Start`.

## Setting it up

1. Copy the **Public Key** from the app's **General Information** page in the Developer Portal. This is `PUBLIC_KEY`.
2. Run the server where Discord can reach it over HTTPS, such as behind a reverse proxy, or through a tunnel while developing.
3. Set **Interactions Endpoint URL** on the same page to the server's URL, such as `https://bot.example.com/interactions`. Discord sends a test request when saving, which the handler must answer, so the server has to be running.

## What the handler does

- verifies each request with the public key, and rejects replayed and stale ones;
- answers Discord's PING;
- runs the same handlers and middleware as gateway interactions, which respond through the same methods;
- answers each request once Discord has the response, or when the 3 seconds pass without one;
- looks up the bot's user with `ctx` for `c.Bot`, since there is no gateway session.

`bot.Shutdown(ctx)` refuses new requests with 503 and waits for running handlers. After it, an HTTP-only bot cannot be started again, as `Open` would connect the gateway.

## Generating an HTTP project

The [CLI](../cli/minidis/README.md) creates one with `minidis init -http mybot`, reading `TOKEN`, `PUBLIC_KEY`, `ADDR` and `GUILD`.

Next: [The Discord client](./client.md).
