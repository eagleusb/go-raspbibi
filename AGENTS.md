# AGENTS.md — go-raspbibi

## Commands

Format:
  go fmt ./...

Fix:
  go fix ./...

Static analysis:
  CGO_ENABLED=0 go vet -tags osusergo ./...

Build (host):
  CGO_ENABLED=0 go build -tags osusergo -o bin/raspbibi .

Build (arm64):
  CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -tags osusergo -o bin/raspbibi-arm64 .

Build all:
  CGO_ENABLED=0 go build -tags osusergo -o bin/raspbibi . && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -tags osusergo -o bin/raspbibi-arm64 .
