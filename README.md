# go-raspbibi

<p align="center">
  <img width="600" height="600" alt="raspbibi-600-600" src="https://github.com/user-attachments/assets/5a1550d9-a75f-489e-92b3-912c7dc8a073" />
</p>

## Transmission

`raspbibi transmission-route` moves newly added torrents via RPC.

### Build and install

Go 1.26+. Choose one build.

```sh
# Native
CGO_ENABLED=0 go build -tags osusergo -o bin/raspbibi .

# Linux ARM64
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -tags osusergo -o bin/raspbibi-arm64 .

# Linux ARMv7
CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -tags osusergo -o bin/raspbibi-armv7 .
```

Replace `bin/raspbibi` below if you cross-built.

```sh
sudo install -m 0755 bin/raspbibi /usr/local/bin/raspbibi
sudo install -d -m 0755 /etc/raspbibi
sudo install -m 0644 examples/transmission-route.json /etc/raspbibi/transmission-route.json
sudo install -m 0755 examples/transmission-route-hook.sh /usr/local/bin/raspbibi-transmission-route
```

#### Enable the hook

For Transmission 4.1.0 beta2, stop the daemon and edit its active `settings.json`.

```sh
sudo systemctl stop transmission-daemon
sudoedit /etc/transmission-daemon/settings.json
```

Merge these settings into the file.

```json
{
  "rpc-enabled": true,
  "script-torrent-added-enabled": true,
  "script-torrent-added-filename": "/usr/local/bin/raspbibi-transmission-route"
}
```

- The launcher runs as the Transmission service account.
- Mount the media filesystems and create writable destination directories.
- Set `rpc_url` in `/etc/raspbibi/transmission-route.json` if the endpoint differs
  from `http://127.0.0.1:9091/transmission/rpc`.

#### RPC authentication

If enabled, put the original RPC password in `/etc/raspbibi/transmission-rpc.env`
(root-owned, mode `0600`).

```ini
TRANSMISSION_RPC_USERNAME=your-user
TRANSMISSION_RPC_PASSWORD=your-password
```

#### Share media access with Jellyfin

Apply group write and setgid permissions to each shared tree, including download
and incomplete directories. This example uses the series directory.

```sh
sudo chgrp -R -- jellyfin /mnt/sda/series
sudo chmod -R -- g+rwX /mnt/sda/series
sudo find /mnt/sda/series -type d -exec chmod g+s -- {} +
```

- Set `"umask": "002"` in Transmission's `settings.json` for group-writable downloads.
- If Jellyfin creates shared files, set `UMask=0002` in its service.

#### Start Transmission

If you configured RPC credentials or shared access, run
`sudo systemctl edit transmission-daemon` and add the applicable settings.

```ini
[Service]
# RPC authentication
EnvironmentFile=/etc/raspbibi/transmission-rpc.env
# Shared media access
SupplementaryGroups=jellyfin
```

```sh
sudo systemctl start transmission-daemon
```

### Routing rules

Edit `/etc/raspbibi/transmission-route.json`. The first matching rule wins.

| Match | Destination |
| --- | --- |
| `Plus.belle.la.vie.encore.plus.belle.S03E123` (season 3, three-digit episode) | `/mnt/sdb/mylen/Plus.belle.la.vie.encore.plus.belle/s03` |
| Series markers such as `S01E02`, `S02`, or `1x02` | `/mnt/sda/series` |
| Year such as `1999` or `2024` | `/mnt/sdb/movies-hd` |

- Put exceptions before general rules and use absolute paths.
- Torrents stay put if no rule matches or the match has an empty or omitted `destination`.

### Preview

```sh
TR_TORRENT_NAME='Example.Show.S01E02.1080p' \
TR_TORRENT_HASH='0123456789abcdef0123456789abcdef01234567' \
./bin/raspbibi transmission-route -config examples/transmission-route.json -dry-run
```

- Dry-run previews the destination without moving data or calling RPC.
- The hook runs once metadata is available and skips torrents already at the destination.
- Relocation moves partial data immediately. Active torrents resume after the move.
- **Requested relocation** means Transmission accepted the request. Check Transmission
  before retrying after a timeout. The default is 30 seconds, adjustable with `-timeout`.

## Chown

Change ownership recursively.

```sh
raspbibi chown -path /srv/media -user jellyfin -group jellyfin -dry-run
```

- Defaults are `-path .`, `-user jellyfin`, and a `-group` matching `-user`.
- Remove `-dry-run` to apply changes. Use `sudo` if needed.

## Mover

```sh
raspbibi mover -src /mnt/sda/downloads -dst /mnt/sdb/movies-hd -dry-run
```

- Finds `.mkv` and `.mp4` files recursively, ignoring case and skipping names
  with season markers such as `S01`.
- Moves files directly into the destination and sanitizes names. `Movie Name (2024).mkv`
  becomes `Movie.Name.2024.mkv`.
- Defaults are `-src .`, `-dst /mnt/sdb/movies-hd`, and `-algo sampling`.
  Use `-algo full` for full-file checksums.
- Keep the destination outside the source tree. On a collision, matching size
  and checksum trigger source deletion. Differing files overwrite the destination.
- Remove `-dry-run` to apply moves.
