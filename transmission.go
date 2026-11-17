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
}

func runTransmissionRoute(ctx context.Context) {
	fs := flag.NewFlagSet("transmission-route", flag.ExitOnError)
	configPath := fs.String("config", "/etc/raspbibi/transmission-route.json", "Routing configuration file")
	dryRun := fs.Bool("dry-run", false, "Show the matched destination without filesystem changes or RPC requests")
	timeout := fs.Duration("timeout", 30*time.Second, "Maximum RPC request duration")
	fs.Parse(os.Args[1:])
	if fs.NArg() != 0 || *timeout <= 0 {
		fs.Usage()
		os.Exit(1)
	}
	if err := executeTransmissionRoute(ctx, *configPath, *dryRun, *timeout); err != nil {
		fmt.Fprintf(os.Stderr, "transmission-route: %v\n", err)
		os.Exit(1)
	}
}

func executeTransmissionRoute(ctx context.Context, configPath string, dryRun bool, timeout time.Duration) error {
	f, err := os.Open(configPath)
	if err != nil {
		return fmt.Errorf("open routing config: %w", err)
	}
	cfg, err := transmission.LoadConfig(f)
	f.Close()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return transmission.Run(ctx, cfg, transmission.Torrent{
		Name: os.Getenv("TR_TORRENT_NAME"),
		Hash: os.Getenv("TR_TORRENT_HASH"),
		Dir:  os.Getenv("TR_TORRENT_DIR"),
	}, transmission.Client{
		Username: os.Getenv("TRANSMISSION_RPC_USERNAME"),
		Password: os.Getenv("TRANSMISSION_RPC_PASSWORD"),
	}, dryRun, os.Stdout)
}
