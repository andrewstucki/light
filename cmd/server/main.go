package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/andrewstucki/light/tunnel"
	"golang.org/x/sync/errgroup"
)

func main() {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	group, ctx := errgroup.WithContext(ctx)
	group.Go(func() error {
		return tunnel.RunServer(ctx, tunnel.ServerConfig{
			Host:     "localhost",
			Address:  "127.0.0.1",
			HTTPPort: 8080,
			GRPCPort: 8081,
		})
	})

	select {
	case <-stop:
		cancel()
	case <-ctx.Done():
	}
	if err := group.Wait(); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
