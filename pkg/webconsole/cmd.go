// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package webconsole

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/qiangli/coreutils/pkg/coopauth"
)

// NewAppsCmd is the `bashy apps` tree.
func NewAppsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "apps",
		Short: "open bashy's apps in a browser: Terminal, Files, Relay, and every declared surface",
		Long: "apps serves bashy's surfaces in a browser at one address.\n\n" +
			"It is ONE launcher with the apps deep-linked beneath it, not one server per\n" +
			"verb: one nav, one auth, one design system. `bashy commands --view web` lists\n" +
			"the same surfaces in the terminal.",
		SilenceUsage: true,
		// The caller (bashy's dispatch arm) prints the error. Without this cobra
		// prints its own copy first and every failure is reported twice.
		SilenceErrors: true,
	}
	cmd.AddCommand(newServeCmd(), newListCmd())
	// Bare `bashy apps` serves — the common case should not need a subcommand.
	cmd.RunE = func(c *cobra.Command, args []string) error {
		serve, _, err := c.Find([]string{"serve"})
		if err != nil {
			return err
		}
		return serve.RunE(serve, args)
	}
	return cmd
}

func newServeCmd() *cobra.Command {
	var (
		port  int
		bind  string
		scope string
		write bool
	)
	cmd := &cobra.Command{
		Use:           "serve",
		Short:         "serve the console",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(c *cobra.Command, _ []string) error {
			return runServe(c.Context(), c.OutOrStdout(), Options{
				Scope:      scope,
				AllowWrite: write,
			}, bind, port)
		},
	}
	cmd.Flags().IntVar(&port, "port", DefaultPort, "port to listen on")
	cmd.Flags().StringVar(&bind, "bind", "127.0.0.1", "address to bind")
	cmd.Flags().StringVar(&scope, "scope", "", "filesystem root for the files panel (default: your home directory)")
	cmd.Flags().BoolVar(&write, "allow-write", false, "allow the files panel to modify files")
	return cmd
}

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "list",
		Short:         "list the apps and whether each one is up",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(c *cobra.Command, _ []string) error {
			var pc probeCache
			w := tabwriter.NewWriter(c.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "SURFACE\tPATH\tMODE\tSTATUS\tSTART")
			for _, st := range pc.Probe(c.Context(), Discover()) {
				note := st.StartHint
				if st.Status == StatusUnavailable {
					note = st.Note
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
					st.Name, st.Path, st.Mode, st.Status, note)
			}
			return w.Flush()
		},
	}
}

// runServe binds and serves. The bind precondition is checked BEFORE listening,
// so an operator who asks for something unsafe learns at the point of the
// mistake rather than from a 403 later.
func runServe(ctx context.Context, out io.Writer, opts Options, bind string, port int) error {
	if ctx == nil {
		ctx = context.Background()
	}
	addr := net.JoinHostPort(bind, strconv.Itoa(port))

	if !coopauth.IsLoopbackAddr(addr) {
		// The terminal panel hands out a shell running as this user and the files
		// panel reads this user's files. Off-loopback that needs a system login,
		// which is not wired yet — so refuse rather than expose it.
		return fmt.Errorf("apps: --bind %s would expose this host's shell and "+
			"files off-loopback, which requires a system login that is not wired yet.\n"+
			"Bind 127.0.0.1 (the default), or reach the console through outpost, which "+
			"authenticates for it", bind)
	}
	opts.Ctx = ctx

	h, closer, err := Handler(opts)
	if err != nil {
		return err
	}
	defer func() { _ = closer() }()

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("apps: listen %s: %w", addr, err)
	}

	url := "http://" + addr + "/"
	fmt.Fprintf(out, "bashy apps: %s\n", url)
	for _, st := range (&probeCache{}).Probe(ctx, Discover()) {
		fmt.Fprintf(out, "  %-9s %s%s\n", st.Status, url[:len(url)-1], st.Path)
	}

	srv := &http.Server{Handler: h, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}()
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
