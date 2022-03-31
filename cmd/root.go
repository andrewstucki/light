package cmd

import (
	"context"
	"fmt"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"

	"github.com/andrewstucki/light/tunnel"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "light",
	Short: "Minimal ngrok clone.",
	Run: func(cmd *cobra.Command, args []string) {
		if localPort == 0 {
			fmt.Fprintln(os.Stderr, "port value must be specified")
			os.Exit(1)
		}
		local, err := url.Parse("http://localhost:" + strconv.Itoa(localPort))
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
		proxy := httputil.NewSingleHostReverseProxy(local)
		if err := tunnel.Connect(context.Background(), tunnel.Config{
			Server:  server,
			ID:      id,
			Handler: proxy,
			Token:   token,
		}); err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
	},
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

var (
	localPort int
	server    string
	id        string
	token     string
)

func init() {
	rootCmd.Flags().IntVarP(&localPort, "port", "p", 0, "Local port to proxy to.")
	rootCmd.Flags().StringVarP(&server, "server", "s", "http://localhost", "Server connection string")
	rootCmd.Flags().StringVarP(&token, "token", "t", "", "Token to use on connect.")
	rootCmd.Flags().StringVarP(&id, "id", "i", "", "id to use for connection")
}
