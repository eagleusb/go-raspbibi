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
	features["route"] = runTransmissionRoute
	features["prune"] = runTransmissionPrune
}

func runTransmissionRoute(ctx context.Context) {
	fs := flag.NewFlagSet("route", flag.ExitOnError)
	dryRun := fs.Bool("dry-run", false, "Show the matched destination without filesystem changes or RPC requests")
	timeout := fs.Duration("timeout", 30*time.Second, "Maximum RPC request duration")
	fs.Parse(os.Args[1:])
	if fs.NArg() != 0 || *timeout <= 0 {
		fs.Usage()
		os.Exit(1)
	}
	if err := executeTransmissionRoute(ctx, *dryRun, *timeout); err != nil {
		fmt.Fprintf(os.Stderr, "route: %v\n", err)
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

func runTransmissionPrune(ctx context.Context) {
	fs := flag.NewFlagSet("prune", flag.ExitOnError)
	dryRun := fs.Bool("dry-run", false, "List expired torrents without removing them")
	timeout := fs.Duration("timeout", 30*time.Second, "Maximum duration of the pruning RPC calls")
	fs.Parse(os.Args[1:])
	if fs.NArg() != 0 || *timeout <= 0 {
		fs.Usage()
		os.Exit(1)
	}
	client, err := newTransmissionClient()
	if err == nil {
		ctx, cancel := context.WithTimeout(ctx, *timeout)
		err = transmission.Prune(ctx, client, *dryRun, os.Stdout)
		cancel()
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "prune: %v\n", err)
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
