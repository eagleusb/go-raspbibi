package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/eagleusb/go-raspbibi/internal/mover"
)

func init() {
	features["mover"] = runMover
}

func runMover(ctx context.Context) {
	fs := flag.NewFlagSet("mover", flag.ExitOnError)
	src := fs.String("src", ".", "Source directory")
	dst := fs.String("dst", "/mnt/sdb/movies-hd", "Destination directory")
	dryRun := fs.Bool("dry-run", false, "Show what would happen without actually moving files")
	algo := fs.String("algo", "sampling", "Checksum algorithm: sampling (default) or full")
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

	if *algo != "sampling" && *algo != "full" {
		fmt.Printf("Invalid algo %q, must be 'sampling' or 'full'\n", *algo)
		fs.Usage()
		os.Exit(1)
	}

	cfg := mover.Config{
		Src:    *src,
		Dst:    *dst,
		Algo:   *algo,
		DryRun: *dryRun,
	}

	if err := mover.Run(ctx, cfg); err != nil {
		fmt.Printf("Error: %v\n", err)
	}
}
