package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/user"
	"strconv"
	"sync"

	"github.com/eagleusb/go-raspbibi/internal/chown"
)

func init() {
	features["chown"] = runChown
}

func runChown(ctx context.Context) {
	fs := flag.NewFlagSet("chown", flag.ExitOnError)
	path := fs.String("path", ".", "Target directory")
	username := fs.String("user", "jellyfin", "Owner username")
	groupname := fs.String("group", "jellyfin", "Shared group name")
	dryRun := fs.Bool("dry-run", false, "Show what would happen without making changes")
	fs.Parse(os.Args[1:])

	if fs.NArg() > 0 {
		fmt.Printf("Unexpected arguments %v\n", fs.Args())
		fs.Usage()
		os.Exit(1)
	}

	if *path == "" {
		fs.Usage()
		os.Exit(1)
	}

	uid, err := lookupChownUID(*username)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	// Resolve GID
	g, err := user.LookupGroup(*groupname)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
	gid, err := strconv.Atoi(g.Gid)
	if err != nil {
		fmt.Printf("Error: invalid GID %q for group %s\n", g.Gid, *groupname)
		os.Exit(1)
	}

	cfg := chown.Config{
		Path:   *path,
		UID:    uid,
		GID:    gid,
		DryRun: *dryRun,
	}
	if *username == "jellyfin" {
		cfg.ResolvePartialUID = sync.OnceValues(func() (int, error) {
			return lookupChownUID("debian-transmission")
		})
	}

	if err := chown.Run(ctx, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func lookupChownUID(username string) (int, error) {
	u, err := user.Lookup(username)
	if err != nil {
		return 0, err
	}
	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return 0, fmt.Errorf("invalid UID %q for user %s", u.Uid, username)
	}
	return uid, nil
}
