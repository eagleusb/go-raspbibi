# go-raspbibi

<p align="center">
  <img width="600" height="600" alt="raspbibi-600-600" src="https://github.com/user-attachments/assets/5a1550d9-a75f-489e-92b3-912c7dc8a073" />
</p>

## Transmission

`raspbibi route` moves newly added torrents via RPC.

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
  "script-torrent-added-filename": "/usr/local/bin/raspbibi"
}
```

- The binary runs as the Transmission service account.
- Mount the media filesystems and create writable destination directories.
- RPC defaults to `http://127.0.0.1:9091/transmission/rpc` without authentication.

#### Optional remote RPC

Set `TRANSMISSION_RPC_URL` for a remote endpoint. For a systemd service, put the
variables in `/etc/raspbibi/transmission.env` (root-owned, mode `0600`).

```ini
TRANSMISSION_RPC_URL=https://transmission.example.com/transmission/rpc
TRANSMISSION_RPC_USERNAME=your-user
TRANSMISSION_RPC_PASSWORD=your-password
```

- Set the username and original password only when the server requires authentication.
  Authenticated RPC requires HTTPS outside loopback.

#### Start Transmission

For shared media, configure [ownership and permissions](#chown) before starting
Transmission. If you configured an RPC environment file, add it with
`sudo systemctl edit transmission-daemon`.

```ini
[Service]
EnvironmentFile=/etc/raspbibi/transmission.env
```

```sh
sudo systemctl start transmission-daemon
```

### Routing rules

Edit the [built-in rules](internal/transmission/rules.go) and rebuild to change routing.
The first matching rule wins.

| Match | Destination |
| --- | --- |
| `Plus.belle.la.vie.encore.plus.belle.S03E123` (season 3, three-digit episode) | `/mnt/sdb/mylen/Plus.belle.la.vie.encore.plus.belle/s03` |
| Series markers such as `S01E02`, `S02`, or `1x02` | `/mnt/sda/series` |
| Year such as `1999` or `2024` | `/mnt/sdb/movies-hd` |

- Put exceptions before general rules and use absolute paths.
- Torrents stay put if no rule matches or the match has an empty destination.

### Preview

```sh
TR_TORRENT_NAME='Example.Show.S01E02.1080p' \
TR_TORRENT_HASH='0123456789abcdef0123456789abcdef01234567' \
./bin/raspbibi route -dry-run
```

- Dry-run previews the destination without moving data or calling RPC.
- The hook runs once metadata is available. It checks the final download directory
  before skipping a torrent whose current directory matches the destination.
- Relocation moves partial data immediately. Active torrents resume after the move.
- **Requested relocation** means Transmission accepted the request. Check Transmission
  before retrying after a timeout. The default is 30 seconds, adjustable with `-timeout`.

### Prune

`raspbibi prune` removes completed torrent entries after ten days
(864,000 seconds) of cumulative seeding. It keeps downloaded files in place.
Transmission's ratio and idle limits can pause a torrent before it reaches the
threshold.

```sh
/usr/local/bin/raspbibi prune -dry-run
/usr/local/bin/raspbibi prune
```

Dry-run reads the torrent list over RPC and prints the entries that would be
removed. The command uses the same `TRANSMISSION_RPC_*` settings as routing and
allows 30 seconds for the run by default. Adjust that with `-timeout`.

To check hourly, add this line to the crontab of an account with RPC access.
Provide any required RPC environment variables to the scheduled command.

```cron
0 * * * * /usr/local/bin/raspbibi prune
```

Cleanup runs independently of the torrent-added routing hook. A torrent becomes
eligible on the first check after it reaches the threshold.

## Chown

Apply shared ownership and permissions to media trees. Jellyfin can read media
and delete entries from writable parent directories.

```sh
sudo /usr/local/bin/raspbibi chown -path /srv/media -dry-run
```

- Defaults are `-path .`, `-user jellyfin`, and `-group jellyfin`.
- With `-user jellyfin`, regular `.part` files belong to `debian-transmission` so
  Transmission can rename them on completion. Keep `"rename-partial-files": true`
  in Transmission's `settings.json` for trees that mix active and completed downloads.
- For a dedicated incomplete directory, run
  `sudo /usr/local/bin/raspbibi chown -path /path/to/incomplete -user debian-transmission`
  separately and keep it outside the Jellyfin ownership sweep.
- Files get owner and group read/write access. Directories also get search access
  and setgid so new entries inherit the group.
- Remove `-dry-run` to apply changes. Runs report changes and errors.
- Give `debian-transmission` access through the `jellyfin` group. If needed, use
  `sudo systemctl edit transmission-daemon` to add `SupplementaryGroups=jellyfin`
  under `[Service]`.
- Set `"umask": "002"` in Transmission's `settings.json` for group-writable
  downloads. If Jellyfin creates shared files, set `UMask=0002` in its service.

> daily cron at 03:00

```cron
0 3 * * * /usr/local/bin/raspbibi chown -path /srv/media
```

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
- Keep the destination outside the source tree. On a collision, the mover removes
  a duplicate source after byte-for-byte verification. Differing files overwrite
  the destination. A failed comparison leaves both files in place.
- Remove `-dry-run` to apply moves.
