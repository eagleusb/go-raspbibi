package mover

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/eagleusb/go-raspbibi/internal/utility"
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
	// Ensure target directory exists
	if cfg.DryRun {
		fmt.Printf("[DRY-RUN] Would create directory: %s\n", cfg.Dst)
	} else {
		if err := os.MkdirAll(cfg.Dst, 0755); err != nil {
			return err
		}
	}

	err := filepath.WalkDir(cfg.Src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		// Check if shutdown was requested
		if ctx.Err() != nil {
			fmt.Println("Stopping file processing (cancelled)...")
			return ctx.Err()
		}

		// Filter by extension
		ext := strings.ToLower(filepath.Ext(path))
		if !utility.HasExtension(ext, DefaultExtensions) {
			if cfg.DryRun {
				fmt.Printf("[DRY-RUN] Ignored (unsupported extension %s): %s\n", ext, d.Name())
			}
			return nil
		}

		filename := d.Name()
		base := strings.TrimSuffix(filename, ext)

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

		// Check if destination already exists
		exists, match, checkErr := ExistsAndMatches(path, destPath, cfg.Algo)
		if checkErr != nil {
			fmt.Printf("Error checking destination %s: %v\n", destPath, checkErr)
		}

		if exists && match && checkErr == nil {
			if cfg.DryRun {
				fmt.Printf("[DRY-RUN] Would skip (identical): %s -> %s\n", filename, destPath)
			} else {
				fmt.Printf("Skipping (identical): %s -> %s\n", filename, destPath)
				os.Remove(path)
			}
			return nil
		}

		if exists && !match && checkErr == nil {
			if cfg.DryRun {
				fmt.Printf("[DRY-RUN] Would overwrite (content differs): %s -> %s\n", filename, destPath)
			} else {
				fmt.Printf("Overwriting (content differs): %s -> %s\n", filename, destPath)
			}
		}

		if cfg.DryRun {
			fmt.Printf("[DRY-RUN] Would move: %s -> %s\n", filename, destPath)
		} else {
			fmt.Printf("Moving: %s -> %s\n", filename, destPath)
			if err := MoveFile(ctx, path, destPath); err != nil {
				fmt.Printf("Error moving %s: %v\n", filename, err)
			}
		}

		return nil
	})

	if err != nil && err != context.Canceled {
		return err
	}

	if cfg.DryRun {
		fmt.Println("File processing completed (dry-run)")
	} else {
		fmt.Println("File processing completed")
	}

	return nil
}
