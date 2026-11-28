# Releasing

The repository holds three Go modules, each tagged on its own:

| Module | Tag |
| --- | --- |
| `github.com/tbdsux/minidis` (the root) | `vX.Y.Z` |
| `github.com/tbdsux/minidis/voice` | `voice/vX.Y.Z` |
| `github.com/tbdsux/minidis/node` | `node/vX.Y.Z` |

`node` requires the other two by version. In the repository, `go.work` builds all three from the checkout, so `node/go.mod` only matters to people who install it. A tag is permanent once the Go module proxy has seen it: never move or delete one, release a new version instead.

Release in this order, because `node` can only require versions that exist:

1. Set `LibraryVersion` in `discord/client/session.go` to the root's new version, and merge that to `main`.
2. Tag the root and, if it changed, `voice` on `main`, then push the tags:

   ```sh
   git tag -a vX.Y.Z -m "# minidis vX.Y.Z"
   git tag -a voice/vX.Y.Z -m "# minidis/voice vX.Y.Z"
   git push origin vX.Y.Z voice/vX.Y.Z
   ```

3. Point `node` at those versions, outside the workspace so that it resolves them like a user would, and merge that to `main`:

   ```sh
   cd node
   go mod edit -require=github.com/tbdsux/minidis@vX.Y.Z -require=github.com/tbdsux/minidis/voice@vX.Y.Z
   GOWORK=off go mod tidy
   GOWORK=off go test ./...
   ```

   `go mod edit` sets the versions without resolving the old ones first, which `go get` does. If `tidy` reports a version as not found, the module proxy has not fetched the new tag yet, or cached a failed fetch: wait until `https://proxy.golang.org/github.com/tbdsux/minidis/voice/@v/vX.Y.Z.info` answers, and run it again.

4. Tag `node` on that commit and push the tag:

   ```sh
   git tag -a node/vX.Y.Z -m "# minidis/node vX.Y.Z"
   git push origin node/vX.Y.Z
   ```

5. Check that it installs: `go install github.com/tbdsux/minidis/node/cmd/minidis-node@vX.Y.Z`, outside the repository.

## First releases

`v0.4.0`, `voice/v0.1.0` and `node/v0.1.0` were the first releases of `voice` and `node`. Before them, `node/go.mod` pointed at the checkout with `replace`, and the `node/v0.1.0` commit dropped those lines. Later releases follow the steps above as they are.
