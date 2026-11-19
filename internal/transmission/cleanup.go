package transmission

import (
	"context"
	"fmt"
	"io"
)

const seedLimitSeconds = 10 * 24 * 60 * 60

type seedTorrent struct {
	Name           string  `json:"name"`
	Hash           string  `json:"hashString"`
	PercentDone    float64 `json:"percentDone"`
	SecondsSeeding int64   `json:"secondsSeeding"`
	Status         *int    `json:"status"`
}

func (torrent seedTorrent) expired() bool {
	const (
		stopped    = 0
		seedQueued = 5
		seeding    = 6
	)
	if torrent.Status == nil || torrent.PercentDone != 1 ||
		torrent.SecondsSeeding < seedLimitSeconds || !torrentHash.MatchString(torrent.Hash) {
		return false
	}
	switch *torrent.Status {
	case stopped, seedQueued, seeding:
		return true
	default:
		return false
	}
}

// Cleanup removes expired torrent entries while preserving their downloaded data.
func Cleanup(ctx context.Context, client Client, dryRun bool, output io.Writer) error {
	torrents, err := client.seedTorrents(ctx)
	if err != nil {
		return fmt.Errorf("list torrents: %w", err)
	}
	var hashes []string
	for _, torrent := range torrents {
		if !torrent.expired() {
			continue
		}
		hashes = append(hashes, torrent.Hash)
		if dryRun {
			fmt.Fprintf(output, "[DRY-RUN] Would remove %q (%s) after %d seconds of seeding\n",
				torrent.Name, torrent.Hash, torrent.SecondsSeeding)
		}
	}
	if len(hashes) == 0 || dryRun {
		return nil
	}
	if err := client.removeTorrents(ctx, hashes); err != nil {
		return fmt.Errorf("remove expired torrents: %w", err)
	}
	fmt.Fprintf(output, "Removed %d torrent entries and kept their downloaded files\n", len(hashes))
	return nil
}
