# Releasing

The repository holds three Go modules, each tagged on its own:

| Module | Tag |
| --- | --- |
| `github.com/tbdsux/minidis` (the root) | `vX.Y.Z` |
| `github.com/tbdsux/minidis/voice` | `voice/vX.Y.Z` |
| `github.com/tbdsux/minidis/node` | `node/vX.Y.Z` |

`node` requires the other two by version. In the repository, `go.work` builds all three from the checkout, so `node/go.mod` only matters to people who install it. Until the first release, `node/go.mod` also points both at the checkout with `replace`, since there is no version to require yet. A tag is permanent once the Go module proxy has seen it: never move or delete one, release a new version instead.

Release in this order, because `node` can only require versions that exist:

1. Set `LibraryVersion` in `discord/client/session.go` to the root's new version, and merge that to `main`.
2. Tag the root and, if it changed, `voice` on `main`, then push the tags:

   ```sh
   git tag vX.Y.Z
   git tag voice/vX.Y.Z
   git push origin vX.Y.Z voice/vX.Y.Z
   ```

3. Point `node` at those versions, outside the workspace so that it resolves them like a user would, and merge that to `main`:

   ```sh
   cd node
   go mod edit -dropreplace=github.com/tbdsux/minidis -dropreplace=github.com/tbdsux/minidis/voice # the first release only
   GOWORK=off go get github.com/tbdsux/minidis@vX.Y.Z github.com/tbdsux/minidis/voice@vX.Y.Z
   GOWORK=off go mod tidy
   GOWORK=off go test ./...
   ```

4. Tag `node` on that commit and push the tag:

   ```sh
   git tag node/vX.Y.Z
   git push origin node/vX.Y.Z
   ```

5. Check that it installs: `go install github.com/tbdsux/minidis/node/cmd/minidis-node@vX.Y.Z`, outside the repository.

## Next release

The modules are versioned independently. The next release is the first of `voice` and `node`:

| Module | Version | Tag |
| --- | --- | --- |
| root | 0.4.0 (`LibraryVersion` is set) | `v0.4.0` |
| `voice` | 0.1.0 | `voice/v0.1.0` |
| `node` | 0.1.0 | `node/v0.1.0` |

None of them is tagged yet. With these versions, steps 2 to 4 are:

```sh
git tag v0.4.0
git tag voice/v0.1.0
git push origin v0.4.0 voice/v0.1.0

cd node
go mod edit -dropreplace=github.com/tbdsux/minidis -dropreplace=github.com/tbdsux/minidis/voice
GOWORK=off go get github.com/tbdsux/minidis@v0.4.0 github.com/tbdsux/minidis/voice@v0.1.0
GOWORK=off go mod tidy
GOWORK=off go test ./...
# merge that to main, then on it:
git tag node/v0.1.0
git push origin node/v0.1.0
```
