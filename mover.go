package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/eagleusb/go-raspbibi/internal/mover"
	"github.com/eagleusb/go-raspbibi/internal/utility"
)

var allowedExtensions = []string{".mkv", ".mp4"}

var skipPatterns = []utility.Pattern{
	utility.NewPattern(`(?i).*S[0-9][0-9].*`),
}

func init() {
	features["mover"] = runMover
}

func runMover(ctx context.Context) {
	fs := flag.NewFlagSet("mover", flag.ExitOnError)
	src := fs.String("src", ".", "Source directory")
	dst := fs.String("dst", "/mnt/sdb/movies-hd", "Destination directory")
	dryRun := fs.Bool("dry-run", false, "Show what would happen without actually moving files")
	fs.Parse(os.Args[1:])

	if fs.NArg() > 0 {
		fmt.Printf("Unexpected arguments %v\n", fs.Args())
		fs.Usage()
		os.Exit(1)
	}

	if *src == "" || *dst == "" {
		fs.Usage()
		os.Exit(1)
	}

	// Ensure target directory exists
	if *dryRun {
		fmt.Printf("[DRY-RUN] Would create directory: %s\n", *dst)
	} else {
		if err := os.MkdirAll(*dst, 0755); err != nil {
			panic(err)
		}
	}

	err := filepath.WalkDir(*src, func(path string, d os.DirEntry, err error) error {
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
		if !utility.HasExtension(ext, allowedExtensions) {
			if *dryRun {
				fmt.Printf("[DRY-RUN] Ignored (unsupported extension %s): %s\n", ext, d.Name())
			}
			return nil
		}

		filename := d.Name()
		base := strings.TrimSuffix(filename, ext)

		// Skip files matching any skip pattern
		if utility.MatchAny(filename, skipPatterns) {
			if *dryRun {
				fmt.Printf("[DRY-RUN] Would skip (matches pattern): %s\n", filename)
			} else {
				fmt.Printf("Skipping (matches pattern): %s\n", filename)
			}
			return nil
		}

		// Sanitize filename
		finalName := utility.FilterUnicode(utility.ReplaceCharacters(base)) + ext
		destPath := filepath.Join(*dst, finalName)

		if *dryRun {
			fmt.Printf("[DRY-RUN] Would move: %s -> %s\n", filename, destPath)
		} else {
			fmt.Printf("Moving: %s -> %s\n", filename, destPath)
			if err := mover.MoveFile(path, destPath); err != nil {
				fmt.Printf("Error moving %s: %v\n", filename, err)
			}
		}

		return nil
	})

	if err != nil && err != context.Canceled {
		fmt.Printf("Error: %v\n", err)
	}

	if *dryRun {
		fmt.Println("File processing completed (dry-run)")
	} else {
		fmt.Println("File processing completed")
	}
}
