package otelcli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/qiangli/coreutils/external/otel/stack"
)

type Options struct {
	DataDir       string
	ProxyPort     int
	ProxyBindAddr string
	OTLPGRPCPort  int
	OTLPHTTPPort  int
}

func NewCommand() *cobra.Command {
	var opts Options
	cmd := &cobra.Command{
		Use:   "otel",
		Short: "Run the embedded OTEL stack",
	}
	cmd.PersistentFlags().StringVar(&opts.DataDir, "data-dir", defaultDataDir(), "storage directory")
	cmd.PersistentFlags().IntVar(&opts.ProxyPort, "port", stack.DefaultProxyPort, "public proxy/UI port")
	cmd.PersistentFlags().StringVar(&opts.ProxyBindAddr, "bind", "127.0.0.1", "proxy bind address")
	cmd.PersistentFlags().IntVar(&opts.OTLPGRPCPort, "otlp-grpc-port", 4317, "OTLP gRPC ingress port; negative allocates ephemerally")
	cmd.PersistentFlags().IntVar(&opts.OTLPHTTPPort, "otlp-http-port", 4318, "OTLP HTTP ingress port; negative allocates ephemerally")

	cmd.AddCommand(&cobra.Command{
		Use:   "serve",
		Short: "Start collector, traces, metrics, logs, dashboards, and alerts",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServe(cmd.Context(), opts)
		},
	})
	return cmd
}

func runServe(ctx context.Context, opts Options) error {
	cfg := &stack.Config{
		ProxyPort:     opts.ProxyPort,
		ProxyBindAddr: opts.ProxyBindAddr,
		OTLPGRPCPort:  opts.OTLPGRPCPort,
		OTLPHTTPPort:  opts.OTLPHTTPPort,
	}
	svc, err := stack.NewService(cfg, opts.DataDir)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := svc.Manager.Start(ctx); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "OTEL at http://%s/\n", svc.HTTPAddr)
	fmt.Fprintf(os.Stdout, "OTLP gRPC at %s\n", svc.CollectorAddr)
	<-ctx.Done()
	return svc.Manager.Stop(context.Background())
}

func defaultDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(os.TempDir(), "otel")
	}
	return filepath.Join(home, ".agents", "otel")
}
