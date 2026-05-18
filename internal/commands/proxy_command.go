package commands

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
	"honch.dev/honch/internal/proxy"
)

func newProxyServeCommand(deps Dependencies) *cobra.Command {
	return &cobra.Command{
		Use:    "proxy-serve",
		Short:  "Run the local sandbox HTTP proxy",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			root, cfg, _, err := loadRuntime(deps)
			if err != nil {
				return err
			}
			controller := proxy.NewController(proxy.ModeOnline)
			handler, err := controller.Handler(cfg.Sandbox.EndpointURL)
			if err != nil {
				return err
			}
			modePath := proxyModePath(root, cfg)
			wrapped := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if data, err := os.ReadFile(modePath); err == nil {
					if mode, err := proxy.ParseMode(stringTrim(data)); err == nil {
						controller.SetMode(mode)
					}
				}
				handler.ServeHTTP(w, r)
			})
			server := &http.Server{
				Addr:              fmt.Sprintf("127.0.0.1:%d", cfg.Ports.Proxy),
				Handler:           wrapped,
				ReadHeaderTimeout: 5 * time.Second,
			}
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "proxy listening on %s\n", server.Addr)
			return server.ListenAndServe()
		},
	}
}
