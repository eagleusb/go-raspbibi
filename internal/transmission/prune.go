package transmission

import (
	"context"
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/hekmon/transmissionrpc/v3"
)

const seedLimit = 10 * 24 * time.Hour
const pruneBatchSize = 100

func expired(torrent transmissionrpc.Torrent) bool {
	if torrent.ID == nil || *torrent.ID < 0 || torrent.Status == nil ||
		torrent.LeftUntilDone == nil || *torrent.LeftUntilDone != 0 ||
		torrent.TimeSeeding == nil || *torrent.TimeSeeding < seedLimit {
		return false
	}
	switch *torrent.Status {
	case transmissionrpc.TorrentStatusStopped, transmissionrpc.TorrentStatusSeedWait, transmissionrpc.TorrentStatusSeed:
		return true
	default:
		return false
	}
}

// Prune removes expired torrent entries while preserving their downloaded data.
func Prune(ctx context.Context, client Client, dryRun bool, output io.Writer) error {
	torrents, err := findExpiredTorrents(ctx, client.rpc, dryRun)
	if err != nil {
		return err
	}
	var ids []int64
	for _, torrent := range torrents {
		ids = append(ids, *torrent.ID)
		if dryRun {
			name := "(unnamed)"
			if torrent.Name != nil {
				name = *torrent.Name
			}
			fmt.Fprintf(output, "[DRY-RUN] Would remove %q (ID %d) after %d seconds of seeding\n",
				name, *torrent.ID, *torrent.TimeSeeding/time.Second)
		}
	}
	if len(ids) == 0 || dryRun {
		return nil
	}
	ctx = context.WithValue(ctx, numericTorrentIDsKey{}, true)
	if err := client.rpc.TorrentRemove(ctx, transmissionrpc.TorrentRemovePayload{
		IDs:             ids,
		DeleteLocalData: false,
	}); err != nil {
		return fmt.Errorf("remove expired torrents: %w", err)
	}
	fmt.Fprintf(output, "Requested removal of %d torrent entries with local data preserved\n", len(ids))
	return nil
}

func findExpiredTorrents(ctx context.Context, rpc *transmissionrpc.Client, includeNames bool) ([]transmissionrpc.Torrent, error) {
	torrents, err := rpc.TorrentGet(ctx, []string{"id"}, nil)
	if err != nil {
		return nil, fmt.Errorf("list torrent IDs: %w", err)
	}
	if torrents == nil {
		return nil, fmt.Errorf("torrent ID response has no torrents array")
	}
	ids := make([]int64, 0, len(torrents))
	for _, torrent := range torrents {
		if torrent.ID != nil && *torrent.ID >= 0 {
			ids = append(ids, *torrent.ID)
		}
	}

	fields := []string{"id", "leftUntilDone", "secondsSeeding", "status"}
	if includeNames {
		fields = append(fields, "name")
	}
	ctx = context.WithValue(ctx, numericTorrentIDsKey{}, true)
	var expiredTorrents []transmissionrpc.Torrent
	for batch := range slices.Chunk(ids, pruneBatchSize) {
		torrents, err := rpc.TorrentGet(ctx, fields, batch)
		if err != nil {
			return nil, fmt.Errorf("read torrent eligibility: %w", err)
		}
		if torrents == nil {
			return nil, fmt.Errorf("torrent eligibility response has no torrents array")
		}
		for _, torrent := range torrents {
			if expired(torrent) {
				expiredTorrents = append(expiredTorrents, torrent)
			}
		}
	}
	return expiredTorrents, nil
}
