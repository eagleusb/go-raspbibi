package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strings"
)

var features = map[string]func(context.Context){}

func featureList() string {
	names := make([]string, 0, len(features))
	for name := range features {
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

func main() {
	var name string
	if len(os.Args) > 1 {
		name = os.Args[1]
		os.Args = os.Args[1:]
	} else if os.Getenv("TR_APP_VERSION") != "" {
		name = "route"
	} else {
		fmt.Println("Usage: raspbibi <feature> [options]")
		fmt.Println("Features:", featureList())
		os.Exit(1)
	}

	run, ok := features[name]
	if !ok {
		fmt.Printf("Unknown feature: %s\n", name)
		fmt.Println("Features:", featureList())
		os.Exit(1)
	}

	// Set up global signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)

	go func() {
		<-sigChan
		fmt.Println("\nInterrupted. Stopping after current operation completes...")
		cancel()
	}()

	run(ctx)
}
