# AGENTS.md — go-raspbibi

## Project layout

- Find subcommand entry points in `chown.go`, `mover.go`, and `transmission.go`.
  Their implementations live under `internal/`.
- Update the README when CLI usage changes. Keep the Transmission hook setup
  and routing table in sync with the Go implementation.
- For Transmission 4.1.0 beta2 behavior, consult the upstream
  [RPC specification](https://github.com/transmission/transmission/blob/4.1.0-beta.2/docs/rpc-spec.md),
  [configuration documentation](https://github.com/transmission/transmission/blob/4.1.0-beta.2/docs/Editing-Configuration-Files.md),
  and [hook implementation](https://github.com/transmission/transmission/blob/4.1.0-beta.2/libtransmission/torrent.cc).

## Development commands

- Format Go code with `go fmt ./...`.
- Apply Go fixers with `go fix ./...` when needed.
- Run tests with `CGO_ENABLED=0 go test -tags osusergo ./...`.
- Run static analysis with `CGO_ENABLED=0 go vet -tags osusergo ./...`.
- Use the host and cross-build commands in [Build and install](README.md#build-and-install).
