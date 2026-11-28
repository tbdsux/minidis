// Command minidis-node runs the audio node.
//
//	MINIDIS_NODE_PASSWORD=secret minidis-node -addr :2333 -local-dir ./music
package main

import (
	"flag"
	"log/slog"
	"net/http"
	"os"

	"github.com/tbdsux/minidis/node"
)

func main() {
	addr := flag.String("addr", ":2333", "listen address")
	localDir := flag.String("local-dir", "", "directory local files may be played from (empty disables local files)")
	flag.Parse()

	password := os.Getenv("MINIDIS_NODE_PASSWORD")
	if password == "" {
		slog.Error("MINIDIS_NODE_PASSWORD must be set")
		os.Exit(2)
	}

	srv := node.New(node.Config{Password: password, Source: &node.FFmpeg{LocalDir: *localDir}})
	slog.Info("minidis-node listening", "addr", *addr, "local_dir", *localDir)
	if err := http.ListenAndServe(*addr, srv); err != nil {
		slog.Error("listen", "error", err)
		os.Exit(1)
	}
}
