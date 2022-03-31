package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/andrewstucki/light/tunnel"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Run the light server.",
	Run: func(cmd *cobra.Command, args []string) {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()

		port := httpPort
		if port == 0 {
			if acmeEmailAddress != "" {
				port = 443
			} else {
				port = 80
			}
		}

		group, ctx := errgroup.WithContext(ctx)
		group.Go(func() error {
			return tunnel.RunServer(ctx, tunnel.ServerConfig{
				Host:             host,
				Address:          address,
				HTTPPort:         port,
				GRPCPort:         grpcPort,
				Token:            serverToken,
				ACMEEmailAddress: acmeEmailAddress,
			})
		})

		if err := group.Wait(); err != nil {
			if !strings.Contains(err.Error(), "context canceled") {
				fmt.Fprintln(os.Stderr, err.Error())
				os.Exit(1)
			}
		}
	},
}

var (
	host             string
	address          string
	acmeEmailAddress string
	serverToken      string
	httpPort         int
	grpcPort         int
)

func init() {
	serverCmd.Flags().StringVarP(&host, "host", "", "localhost", "Server host.")
	serverCmd.Flags().StringVarP(&address, "address", "a", "127.0.0.1", "Bind address for server.")
	serverCmd.Flags().StringVarP(&acmeEmailAddress, "enable-acme-email", "", "", "ACME email address to use (enables TLS).")
	serverCmd.Flags().StringVarP(&serverToken, "token", "t", "", "Token to have basic auth on connect.")
	serverCmd.Flags().IntVarP(&httpPort, "http", "", 0, "HTTP port, defaults to 80 or 443 if TLS is enabled.")
	serverCmd.Flags().IntVarP(&grpcPort, "grpc", "", 8443, "GRPC port.")

	rootCmd.AddCommand(serverCmd)
}
