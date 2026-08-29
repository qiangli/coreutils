// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package webconsole

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/qiangli/coreutils/pkg/coopauth"
	"github.com/qiangli/coreutils/pkg/hostauth"
	"github.com/qiangli/coreutils/pkg/websession"
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
	cmd.AddCommand(newServeCmd(), newListCmd(), newServiceCmd())
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

// sessionKey returns the persistent HMAC key for console sessions, creating it
// on first use with 0600 permissions.
func sessionKey() ([]byte, error) {
	dir := os.Getenv("BASHY_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		dir = filepath.Join(home, ".bashy")
	}
	dir = filepath.Join(dir, "console")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "session.key")
	if b, err := os.ReadFile(path); err == nil && len(b) >= 32 {
		return b, nil
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, key, 0o600); err != nil {
		return nil, err
	}
	return key, nil
}

func newServeCmd() *cobra.Command {
	var (
		port    int
		bind    string
		scope   string
		write   bool
		disable []string
		apps    []string
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
				Disable:    disable,
				Apps:       apps,
			}, bind, port)
		},
	}
	cmd.Flags().IntVar(&port, "port", DefaultPort, "port to listen on")
	cmd.Flags().StringVar(&bind, "bind", "127.0.0.1", "address to bind")
	cmd.Flags().StringVar(&scope, "scope", "", "filesystem root for the files panel (default: your home directory)")
	cmd.Flags().BoolVar(&write, "allow-write", false, "allow the files panel to modify files")
	cmd.Flags().StringSliceVar(&disable, "disable", nil,
		"panels to leave out entirely — neither listed nor routed (terminal,files,relay,mb,board)")
	cmd.Flags().StringArrayVar(&apps, "app", nil,
		"publish a third-party program as a tile: <bin> or <bin>@<port>, repeatable "+
			"(described by `<bin> meta --json`)")
	return cmd
}

func newListCmd() *cobra.Command {
	var apps []string
	cmd := &cobra.Command{
		Use:           "list",
		Short:         "list the apps and whether each one is up",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(c *cobra.Command, _ []string) error {
			var pc probeCache
			panels := Discover()
			if len(apps) > 0 {
				extra, errs := discoverApps(c.Context(), apps, nil, TakenMounts(panels))
				for _, err := range errs {
					fmt.Fprintf(c.ErrOrStderr(), "apps: skipping --app: %v\n", err)
				}
				panels = append(panels, extra...)
			}
			w := tabwriter.NewWriter(c.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "SURFACE\tPATH\tMODE\tAUTH\tSOURCE\tSTATUS\tSTART")
			for _, st := range pc.Probe(c.Context(), panels) {
				note := st.StartHint
				if st.Status == StatusUnavailable {
					note = st.Note
				}
				auth := st.Auth
				if auth == "" {
					auth = AuthSystem
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
					st.Name, st.Path, st.Mode, auth, st.Source, st.Status, note)
			}
			return w.Flush()
		},
	}
	cmd.Flags().StringArrayVar(&apps, "app", nil,
		"publish a third-party program as a tile: <bin> or <bin>@<port>, repeatable")
	return cmd
}

// runServe binds and serves. The bind precondition is checked BEFORE listening,
// so an operator who asks for something unsafe learns at the point of the
// mistake rather than from a 403 later.
func runServe(ctx context.Context, out io.Writer, opts Options, bind string, port int) error {
	if ctx == nil {
		ctx = context.Background()
	}
	addr := net.JoinHostPort(bind, strconv.Itoa(port))

	// Binding off-loopback exposes this host's shell and files, so it requires a
	// system login. The check is at BIND time, not per request: an operator who
	// asks for something that cannot be made safe should learn at the point of
	// the mistake, not from a 403 an hour later.
	if !coopauth.IsLoopbackAddr(addr) {
		auth := hostauth.DefaultAuthenticator()
		// Probe with empty credentials: every backend reports "I work, and no,
		// that is not a password" without needing a real secret.
		if err := auth.Authenticate("", ""); !errors.Is(err, hostauth.ErrInvalidCredentials) {
			return fmt.Errorf("apps: --bind %s exposes this host's shell and files "+
				"off-loopback, which requires a system login, and host authentication "+
				"is not available here (%w).\nBind 127.0.0.1, or reach it through outpost",
				bind, err)
		}
		opts.RequireLogin = true

		// Persist the signing key so a restart does not sign everyone out.
		key, err := sessionKey()
		if err != nil {
			return fmt.Errorf("apps: session key: %w", err)
		}
		opts.Sessions = websession.NewStore(12*time.Hour, key)
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
