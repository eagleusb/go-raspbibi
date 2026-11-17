package transmission

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

var torrentHash = regexp.MustCompile(`(?i)^[0-9a-f]{40}$`)

type Torrent struct {
	Name string
	Hash string
	Dir  string
}

// Run requests relocation of just the hook's torrent. It never renames torrent
// contents, stops/starts torrents, or uses the standalone filesystem mover.
func Run(ctx context.Context, cfg Config, torrent Torrent, client Client, dryRun bool, output io.Writer) error {
	if torrent.Name == "" {
		return fmt.Errorf("TR_TORRENT_NAME is required")
	}
	if !torrentHash.MatchString(torrent.Hash) {
		return fmt.Errorf("TR_TORRENT_HASH must be a 40-character hexadecimal info hash")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	rule, matched := cfg.Match(torrent.Name)
	if !matched || rule.Destination == "" {
		fmt.Fprintf(output, "Keeping %q in its existing download folder\n", torrent.Name)
		return nil
	}
	destination := filepath.Clean(rule.Destination)
	if torrent.Dir != "" && filepath.Clean(torrent.Dir) == destination {
		fmt.Fprintf(output, "Keeping %q: already in %q\n", torrent.Name, destination)
		return nil
	}
	if dryRun {
		fmt.Fprintf(output, "[DRY-RUN] Would request relocation of %q to %q (rule %q)\n", torrent.Name, destination, rule.Name)
		return nil
	}
	// Require an existing destination: do not create folders on the root disk
	// when one of the Raspberry Pi's external disks is unavailable.
	info, err := os.Stat(destination)
	if err != nil {
		return fmt.Errorf("destination %q is unavailable: %w", destination, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("destination %q is not a directory", destination)
	}
	client.URL = cfg.RPCURL
	if err := client.Move(ctx, torrent.Hash, destination); err != nil {
		return err
	}
	fmt.Fprintf(output, "Requested relocation of %q to %q (rule %q)\n", torrent.Name, destination, rule.Name)
	return nil
}
