# go-raspbibi

<p align="center">
  <img width="600" height="600" alt="raspbibi-600-600" src="https://github.com/user-attachments/assets/5a1550d9-a75f-489e-92b3-912c7dc8a073" />
</p>

## Transmission routing hook

`raspbibi transmission-route` chooses a torrent's download directory from its
name. It reads Transmission's `TR_TORRENT_NAME`, `TR_TORRENT_HASH`, and
`TR_TORRENT_DIR` hook environment variables and asks Transmission to relocate
the torrent using RPC. It does not rename files or use the standalone `mover`
feature, so Transmission keeps track of its data for downloading and seeding.

The supplied configuration uses these rules, in order:

| Match | Destination |
| --- | --- |
| A `custom` word marker (case insensitive) | Keep the existing download folder |
| Series markers such as `S01E02`, `S02`, or `1x02` | `/mnt/sda/séries` |
| A year such as `1999` or `2024` | `/mnt/sdb/movies-hd` |
| No matching rule | Keep the existing download folder |

The first match wins. Edit `examples/transmission-route.json` to match your
naming conventions: years are only a movie heuristic, and daily episodes,
anime, and titles without years may need more specific rules. Put custom
exceptions before series and movies. Any rule with an omitted or empty
`destination` keeps that torrent in its existing folder, even if a later rule
would match. Paths must be absolute; Unicode paths are supported. There is no
fallback relocation for unknown torrents.

### Build and install on Raspbian / Debian

Build with Go 1.26 or newer. For a native build:

```sh
CGO_ENABLED=0 go build -tags osusergo -o bin/raspbibi .
```

For cross compilation, choose the target for your **installed OS**, rather
than just the Raspberry Pi model:

```sh
# 64-bit Raspberry Pi OS
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -tags osusergo -o bin/raspbibi-arm64 .

# 32-bit Raspbian on an ARMv7-compatible Pi
CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -tags osusergo -o bin/raspbibi-armv7 .
```

Install the appropriate binary on the Pi (substitute its filename below if
cross compiled):

```sh
sudo install -m 0755 bin/raspbibi /usr/local/bin/raspbibi
sudo install -d -m 0755 /etc/raspbibi
sudo install -m 0644 examples/transmission-route.json /etc/raspbibi/transmission-route.json
sudo install -m 0755 examples/transmission-route-hook.sh /usr/local/bin/raspbibi-transmission-route
```

The movie and series destination directories must already exist on the mounted
disks and be writable by the Transmission service account (often
`debian-transmission`). The hook refuses a missing destination instead of
creating it. This does not verify that a disk is mounted: ensure the mounts are
available before starting Transmission.

### Enable the added hook in Transmission 4.1.0 beta2

Stop Transmission before editing its existing `settings.json`. On a typical
Debian daemon installation this is `/etc/transmission-daemon/settings.json`;
some installations instead use a file under `/var/lib/transmission-daemon`.
Merge these entries into that file, preserving its other settings:

```json
"rpc-enabled": true,
"script-torrent-added-enabled": true,
"script-torrent-added-filename": "/usr/local/bin/raspbibi-transmission-route"
```

For a systemd daemon:

```sh
sudo systemctl stop transmission-daemon
sudoedit /etc/transmission-daemon/settings.json
sudo systemctl start transmission-daemon
```

The small shell launcher is required because Transmission's hook setting takes
only an executable filename. All matching and RPC logic runs in Go. You may
set `RASPBIBI_TRANSMISSION_CONFIG` in the service environment to choose another
configuration file. With the GTK client, close the app and edit its settings
(typically `~/.config/transmission/settings.json`), then reopen it; enable its
local RPC server as well.

The default RPC URL is `http://127.0.0.1:9091/transmission/rpc`; change
`rpc_url` in the routing configuration if the daemon uses another endpoint.
This implementation uses the legacy RPC protocol supported by **4.1.0 beta2**,
including the HTTP 409 session-token handshake. No `transmission-remote`
installation or additional Go dependency is required. Existing RPC access
controls still apply.

If RPC authentication is enabled, set `TRANSMISSION_RPC_USERNAME` and
`TRANSMISSION_RPC_PASSWORD` in the Transmission service's environment so the
hook inherits them. For systemd, create a root-readable environment file such
as `/etc/raspbibi/transmission-rpc.env` with mode `0600`:

```ini
TRANSMISSION_RPC_USERNAME=your-user
TRANSMISSION_RPC_PASSWORD=your-password
```

Use `sudo systemctl edit transmission-daemon` to add:

```ini
[Service]
EnvironmentFile=/etc/raspbibi/transmission-rpc.env
```

Restart the service after changing its environment. Do not put credentials in
`rpc_url` or commit them with the routing rules.

### Preview and behavior

Preview the routing decision without making RPC requests or filesystem changes:

```sh
TR_TORRENT_NAME='Example.Show.S01E02.1080p' \
TR_TORRENT_HASH='0123456789abcdef0123456789abcdef01234567' \
./bin/raspbibi transmission-route -config examples/transmission-route.json -dry-run
```

The hook applies to newly added torrents, not the existing queue. In beta2 it
runs once metadata is available, including for magnet links. Relocation uses
`torrent-set-location` with `move: true`, scoped to that torrent's info hash.
On success it prints **Requested relocation**: RPC acceptance does not confirm
that a potentially long disk move has finished. Inspect Transmission for the
final location or move errors. The request timeout defaults to 30 seconds and
can be changed with `-timeout 2m`. A timeout can leave the outcome uncertain;
check Transmission before retrying.

Transmission can briefly interrupt uploading while moving data; after a
successful move an active torrent continues automatically. A move failure can
stop the torrent. Explicitly paused torrents are not started by the hook.
Relocation also overrides Transmission's incomplete-directory setting: partial
data moves into the category directory immediately. Unknown and custom
torrents retain their current location, including any manually chosen folder.
An already matching `TR_TORRENT_DIR` is skipped.

The hook exits nonzero for invalid configuration, missing hook variables,
unavailable destinations, authentication failures, or RPC errors. Check the
service logs and Transmission's own status when troubleshooting.

### Validation

```sh
go fmt ./...
go fix ./...
CGO_ENABLED=0 go test -tags osusergo ./...
CGO_ENABLED=0 go vet -tags osusergo ./...
```

Protocol and hook behavior are based on Transmission's
[beta2 RPC specification](https://github.com/transmission/transmission/blob/4.1.0-beta.2/docs/rpc-spec.md),
[beta2 configuration documentation](https://github.com/transmission/transmission/blob/4.1.0-beta.2/docs/Editing-Configuration-Files.md),
and [beta2 hook implementation](https://github.com/transmission/transmission/blob/4.1.0-beta.2/libtransmission/torrent.cc).
