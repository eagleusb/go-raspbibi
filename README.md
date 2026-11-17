# go-raspbibi

<p align="center">
  <img width="600" height="600" alt="raspbibi-600-600" src="https://github.com/user-attachments/assets/5a1550d9-a75f-489e-92b3-912c7dc8a073" />
</p>

## Transmission

`raspbibi transmission-route` reads `TR_TORRENT_NAME`, `TR_TORRENT_HASH`, and
`TR_TORRENT_DIR` from Transmission's hook environment and asks Transmission to
move the torrent via RPC. The first matching rule wins.

| Match | Destination |
| --- | --- |
| `Plus.belle.la.vie.encore.plus.belle.S03E123` (season 3, three-digit episode) | `/mnt/sdb/mylen/Plus.Belle.La.Vie.Encore.Plus.Belle/s03` |
| Series markers such as `S01E02`, `S02`, or `1x02` | `/mnt/sda/series/<Show.Title>/sNN` |
| A year such as `1999` or `2024` | `/mnt/sdb/movies-hd` |
| No matching rule | Keep the existing download folder |

The series rules use the title before the season marker. They join words with
dots and capitalize each word while preserving its remaining casing.

- `Orange.Is.the.New.Black.S01.MULTi.1080p.NF.WEB.DDP.AV1-BTT` goes to
  `/mnt/sda/series/Orange.Is.The.New.Black/s01`.
- `MobLand.S02E09.MULTi.4KLight.DV.HDR10+.WEBRip.DDP5.1.Atmos.x265-[BATGirl].mkv`
  goes to `/mnt/sda/series/MobLand/s02`.

Edit `examples/transmission-route.json` to put exceptions before the general
series and year rules. To enable this layout in an installed config, set
`"series_subdirs": true` and set `destination` to the absolute series root.
Use `/mnt/sdb/mylen` for the `Plus belle la vie` rule and `/mnt/sda/series` for
the general series rule. For fixed rules, an empty or omitted `destination`
keeps the current folder.

### Build and install

Use Go 1.26 or newer. Choose the build command for the OS installed on the Pi.

```sh
# Native build
CGO_ENABLED=0 go build -tags osusergo -o bin/raspbibi .

# Cross-compile for 64-bit Raspberry Pi OS
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -tags osusergo -o bin/raspbibi-arm64 .

# Cross-compile for 32-bit Raspbian on an ARMv7-compatible Pi
CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -tags osusergo -o bin/raspbibi-armv7 .
```

On the Pi, install the binary, routing config, and hook launcher. Substitute
the binary filename if you cross-compiled.

```sh
sudo install -m 0755 bin/raspbibi /usr/local/bin/raspbibi
sudo install -d -m 0755 /etc/raspbibi
sudo install -m 0644 examples/transmission-route.json /etc/raspbibi/transmission-route.json
sudo install -m 0755 examples/transmission-route-hook.sh /usr/local/bin/raspbibi-transmission-route
```

Mount the disks before starting Transmission. The hook creates missing series
roots, show folders, and season folders. Fixed destinations must already
exist. Give the service account (often `debian-transmission`) write access to
the destinations and the parent directories where it creates folders.

To enable the hook in Transmission 4.1.0 beta2, run
`sudo systemctl stop transmission-daemon`, then merge these entries into the
existing `settings.json` with `sudoedit`. On Debian, the file is usually
`/etc/transmission-daemon/settings.json` or under `/var/lib/transmission-daemon`.

```json
"rpc-enabled": true,
"script-torrent-added-enabled": true,
"script-torrent-added-filename": "/usr/local/bin/raspbibi-transmission-route"
```

The default RPC endpoint is `http://127.0.0.1:9091/transmission/rpc`. Set
`rpc_url` in the routing config to use another endpoint, or set
`RASPBIBI_TRANSMISSION_CONFIG` in the service environment to select another
config file.

If RPC authentication is enabled, create a root-owned
`/etc/raspbibi/transmission-rpc.env` with mode `0600`. Use the original password
because Transmission saves `rpc-password` as a salted hash.

```ini
TRANSMISSION_RPC_USERNAME=your-user
TRANSMISSION_RPC_PASSWORD=your-password
```

Run `sudo systemctl edit transmission-daemon` and add the environment file.

```ini
[Service]
EnvironmentFile=/etc/raspbibi/transmission-rpc.env
```

Run `sudo systemctl start transmission-daemon` after setup. Restart the service
after later changes to its environment.

For GTK, close the app, merge the same settings into its `settings.json`
(typically `~/.config/transmission/settings.json`), and reopen it.

### Preview and behavior

Use `-dry-run` to preview the destination without moving data or calling RPC.

```sh
TR_TORRENT_NAME='Example.Show.S01E02.1080p' \
TR_TORRENT_HASH='0123456789abcdef0123456789abcdef01234567' \
./bin/raspbibi transmission-route -config examples/transmission-route.json -dry-run
```

- The hook runs for newly added torrents once metadata is available, including
  magnet links. It skips torrents already at the matching destination.
- The hook calls `torrent-set-location` with `move: true` for the torrent's
  info hash. Partial data moves immediately, even with an incomplete directory
  configured. Active torrents resume after a successful move. Paused torrents
  stay paused. Season packs keep their existing file and folder structure.
- **Requested relocation** means Transmission accepted the request. Check
  Transmission for the final location or move errors, especially before retrying
  after a timeout. RPC requests time out after 30 seconds by default. Use
  `-timeout 2m` to override it.
- Check the service logs if the hook exits nonzero for configuration,
  destination, authentication, or RPC errors.

## Chown

`raspbibi chown` recursively changes ownership of a directory and its contents.
It skips entries that already have the requested owner and group.

```sh
raspbibi chown -path /srv/media -user jellyfin -group jellyfin -dry-run
```

`-path` defaults to the current directory, `-user` defaults to `jellyfin`, and
`-group` defaults to a group with the same name as `-user`. Remove `-dry-run` to
apply the changes and use `sudo` if needed.

## Mover

`raspbibi mover` scans a source directory recursively for `.mkv` and `.mp4`
files (case-insensitive) and skips names containing a marker such as `S01`.
It moves files directly into the destination and sanitizes names, for example
`Movie Name (2024).mkv` becomes `Movie.Name.2024.mkv`.

```sh
raspbibi mover -src /mnt/sda/downloads -dst /mnt/sdb/movies-hd -dry-run
```

`-src` defaults to the current directory and `-dst` to `/mnt/sdb/movies-hd`.
Remove `-dry-run` to apply the moves. Keep the destination outside the source
tree so the mover cannot mistake a source file for its own duplicate.

On a name collision, the mover compares file sizes and checksums. It deletes
the source if they match and overwrites the destination if they differ.
`-algo sampling` is the default. Use `-algo full` to checksum entire files.
