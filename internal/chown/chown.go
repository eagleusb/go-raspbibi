package chown

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// Config holds the configuration for a chown run.
type Config struct {
	Path   string
	UID    int
	GID    int
	DryRun bool
}

// Run recursively changes ownership of all files and directories
// under cfg.Path to the specified UID/GID.
func Run(ctx context.Context, cfg Config) error {
	err := filepath.WalkDir(cfg.Path, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Check if shutdown was requested
		if ctx.Err() != nil {
			fmt.Println("Stopping file processing (cancelled)...")
			return ctx.Err()
		}

		// Check current ownership
		var stat unix.Stat_t
		if err := unix.Stat(path, &stat); err != nil {
			return fmt.Errorf("failed to stat %s: %w", path, err)
		}

		// Skip entries that already have the correct owner
		if int(stat.Uid) == cfg.UID && int(stat.Gid) == cfg.GID {
			return nil
		}

		if cfg.DryRun {
			fmt.Printf("[DRY-RUN] Would chown: %s (%d:%d)\n", path, cfg.UID, cfg.GID)
		} else {
			fmt.Printf("Chown: %s (%d:%d)\n", path, cfg.UID, cfg.GID)
			if err := os.Lchown(path, cfg.UID, cfg.GID); err != nil {
				return fmt.Errorf("failed to chown %s: %w", path, err)
			}
		}

		return nil
	})

	if err != nil && err != context.Canceled {
		return err
	}

	if cfg.DryRun {
		fmt.Println("Chown completed (dry-run)")
	} else {
		fmt.Println("Chown completed")
	}

	return nil
}
