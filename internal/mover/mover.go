package mover

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/eagleusb/go-raspbibi/internal/utility"
	"golang.org/x/sys/unix"
)

// Config holds the configuration for a mover run.
type Config struct {
	Src    string
	Dst    string
	Algo   string
	DryRun bool
}

// DefaultExtensions defines the file extensions eligible for moving.
var DefaultExtensions = []string{".mkv", ".mp4"}

// DefaultSkipPatterns defines filename patterns to skip.
var DefaultSkipPatterns = []utility.Pattern{
	utility.NewPattern(`(?i).*S[0-9][0-9].*`),
}

// Run executes the mover feature: walks the source directory, filters,
// sanitizes, and moves matching files to the destination.
func Run(ctx context.Context, cfg Config) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	// Ensure target directory exists
	if cfg.DryRun {
		fmt.Printf("[DRY-RUN] Would create directory: %s\n", cfg.Dst)
	} else {
		if err := os.MkdirAll(cfg.Dst, 0755); err != nil {
			return err
		}
	}

	var failures []error
	err := filepath.WalkDir(cfg.Src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		// Check if shutdown was requested
		if err := ctx.Err(); err != nil {
			return err
		}

		// Filter by extension
		originalExt := filepath.Ext(d.Name())
		ext := strings.ToLower(originalExt)
		if !utility.HasExtension(ext, DefaultExtensions) {
			if cfg.DryRun {
				fmt.Printf("[DRY-RUN] Ignored (unsupported extension %s): %s\n", ext, d.Name())
			}
			return nil
		}

		filename := d.Name()
		base := strings.TrimSuffix(filename, originalExt)

		// Skip files matching any skip pattern
		if utility.MatchAny(filename, DefaultSkipPatterns) {
			if cfg.DryRun {
				fmt.Printf("[DRY-RUN] Would skip (matches pattern): %s\n", filename)
			} else {
				fmt.Printf("Skipping (matches pattern): %s\n", filename)
			}
			return nil
		}

		// Sanitize filename
		finalName := utility.FilterUnicode(utility.ReplaceCharacters(base)) + ext
		destPath := filepath.Join(cfg.Dst, finalName)

		if err := processFile(ctx, cfg, path, destPath); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			failures = append(failures, fmt.Errorf("%s -> %s: %w", path, destPath, err))
		}

		return nil
	})

	if err := errors.Join(append(failures, err)...); err != nil {
		return err
	}

	if cfg.DryRun {
		fmt.Println("File processing completed (dry-run)")
	} else {
		fmt.Println("File processing completed")
	}

	return nil
}

func processFile(ctx context.Context, cfg Config, path, destPath string) error {
	src, err := openRegular(path)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := openRegular(destPath)
	// Only an absent destination from Lstat is safe to treat as a new file.
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("check destination: %w", err)
	}
	var dstInfo os.FileInfo
	match := false
	if dst != nil {
		defer dst.Close()
		dstInfo = dst.info
		match, err = filesMatch(ctx, src, dst, cfg.Algo)
		if err != nil {
			return fmt.Errorf("compare files: %w", err)
		}
	}

	if cfg.DryRun {
		switch {
		case match:
			fmt.Printf("[DRY-RUN] Would remove duplicate: %s (matches %s)\n", path, destPath)
		case dst != nil:
			fmt.Printf("[DRY-RUN] Would overwrite: %s -> %s\n", path, destPath)
		default:
			fmt.Printf("[DRY-RUN] Would move: %s -> %s\n", path, destPath)
		}
		return nil
	}
	if match {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := checkUnchanged(path, src.info); err != nil {
			return err
		}
		if err := checkUnchanged(destPath, dstInfo); err != nil {
			return err
		}
		if err := unix.Unlink(path); err != nil {
			return fmt.Errorf("remove duplicate: %w", err)
		}
		fmt.Printf("Removed duplicate: %s (matches %s)\n", path, destPath)
		return nil
	}
	if dst != nil {
		fmt.Printf("Overwriting (content differs): %s -> %s\n", path, destPath)
	} else {
		fmt.Printf("Moving: %s -> %s\n", path, destPath)
	}
	return moveFile(ctx, src, destPath, dstInfo)
}
