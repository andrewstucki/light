package cmd

import (
	"context"
	"fmt"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/andrewstucki/light/tunnel"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

const (
	defaultConfigFilename = "config"
	envPrefix             = "LIGHT"
)

var rootCmd = &cobra.Command{
	Use:   "light",
	Short: "Minimal ngrok clone.",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return initializeConfig(cmd)
	},
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

func initializeConfig(cmd *cobra.Command) error {
	v := viper.New()
	v.SetConfigType("toml")
	v.SetConfigName(defaultConfigFilename)
	v.AddConfigPath("$HOME/.light")
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return err
		}
	}

	v.SetEnvPrefix(envPrefix)
	v.AutomaticEnv()
	bindFlags(cmd, v)

	return nil
}

func bindFlags(cmd *cobra.Command, v *viper.Viper) {
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if strings.Contains(f.Name, "-") {
			envVarSuffix := strings.ToUpper(strings.ReplaceAll(f.Name, "-", "_"))
			v.BindEnv(f.Name, fmt.Sprintf("%s_%s", envPrefix, envVarSuffix))
		}
		if !f.Changed && v.IsSet(f.Name) {
			val := v.Get(f.Name)
			cmd.Flags().Set(f.Name, fmt.Sprintf("%v", val))
		}
	})
}
