# AGENTS.md — go-raspbibi

## Commands

Format:
  go fmt ./...

Fix:
  go fix ./...

Static analysis:
  go vet ./...

Build (host):
  go build -o bin/raspbibi .

Build (arm64):
  GOOS=linux GOARCH=arm64 go build -o bin/raspbibi-arm64 .

Build all:
  go build -o bin/raspbibi . && GOOS=linux GOARCH=arm64 go build -o bin/raspbibi-arm64 .

## Structure

main.go                       Entry point, feature dispatcher, signal handler.
mover.go                      "mover" feature (walk, filter, sanitize, move, dry-run).
internal/mover/fs.go          Cross-device file move.
internal/utility/filter.go    Pattern matching and extension check.
internal/utility/sanitize.go  Filename sanitization.
