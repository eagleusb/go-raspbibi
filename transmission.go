package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/eagleusb/go-raspbibi/internal/transmission"
)

func init() {
	features["transmission-route"] = runTransmissionRoute
	features["transmission-cleanup"] = runTransmissionCleanup
}

func runTransmissionRoute(ctx context.Context) {
	fs := flag.NewFlagSet("transmission-route", flag.ExitOnError)
	dryRun := fs.Bool("dry-run", false, "Show the matched destination without filesystem changes or RPC requests")
	timeout := fs.Duration("timeout", 30*time.Second, "Maximum RPC request duration")
	fs.Parse(os.Args[1:])
	if fs.NArg() != 0 || *timeout <= 0 {
		fs.Usage()
		os.Exit(1)
	}
	if err := executeTransmissionRoute(ctx, *dryRun, *timeout); err != nil {
		fmt.Fprintf(os.Stderr, "transmission-route: %v\n", err)
		os.Exit(1)
	}
}

func executeTransmissionRoute(ctx context.Context, dryRun bool, timeout time.Duration) error {
	client, err := newTransmissionClient()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return transmission.Run(ctx, transmission.Torrent{
		Name: os.Getenv("TR_TORRENT_NAME"),
		Hash: os.Getenv("TR_TORRENT_HASH"),
		Dir:  os.Getenv("TR_TORRENT_DIR"),
	}, client, dryRun, os.Stdout)
}

func runTransmissionCleanup(ctx context.Context) {
	fs := flag.NewFlagSet("transmission-cleanup", flag.ExitOnError)
	dryRun := fs.Bool("dry-run", false, "List expired torrents without removing them")
	timeout := fs.Duration("timeout", 30*time.Second, "Maximum duration of the cleanup RPC calls")
	fs.Parse(os.Args[1:])
	if fs.NArg() != 0 || *timeout <= 0 {
		fs.Usage()
		os.Exit(1)
	}
	client, err := newTransmissionClient()
	if err == nil {
		ctx, cancel := context.WithTimeout(ctx, *timeout)
		err = transmission.Cleanup(ctx, client, *dryRun, os.Stdout)
		cancel()
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "transmission-cleanup: %v\n", err)
		os.Exit(1)
	}
}

func newTransmissionClient() (transmission.Client, error) {
	return transmission.NewClient(
		os.Getenv("TRANSMISSION_RPC_URL"),
		os.Getenv("TRANSMISSION_RPC_USERNAME"),
		os.Getenv("TRANSMISSION_RPC_PASSWORD"),
	)
}
