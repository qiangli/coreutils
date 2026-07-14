package foreman

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/qiangli/coreutils/pkg/agentlaunch"
)

func (s *Session) ServeControl(ctx context.Context, ready chan<- string) error {
	path := s.store.CtlSockPath()
	if err := s.store.Ensure(); err != nil {
		return err
	}
	_ = os.Remove(path)
	ln, err := net.Listen("unix", path)
	if err != nil {
		return err
	}
	defer func() {
		_ = ln.Close()
		_ = os.Remove(path)
	}()
	if ready != nil {
		ready <- path
	}
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		// CONCURRENTLY. A turn can run for many minutes, and Apply holds the session
		// for all of it. Handling connections inline meant the listener stopped
		// accepting the moment an agent started working — so the one time you most
		// need to say "stop, wrong file", the socket would not even take the call.
		go s.handleControlConn(ctx, conn)
		if s.stopped() {
			return nil
		}
	}
}

func (s *Session) stopped() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state.Stopped
}

func (s *Session) handleControlConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	sc := bufio.NewScanner(conn)
	for sc.Scan() {
		var cmd Command
		if err := json.Unmarshal(sc.Bytes(), &cmd); err != nil {
			fmt.Fprintf(conn, `{"ok":false,"error":%q}`+"\n", err.Error())
			continue
		}

		// A STEER to an agent that is already working goes STRAIGHT to it — it does
		// not queue behind the turn it is trying to interrupt.
		//
		// This is the fast path, and it is the only one that makes `tell` mean what
		// it says. Routing it through Apply would block on the session mutex until
		// the turn finished, at which point the "interruption" arrives as a fresh
		// instruction to an agent that has already done the wrong thing.
		if strings.EqualFold(strings.TrimSpace(cmd.Verb), CommandTell) {
			if steered, err := s.TrySteer(cmd.Message); err != nil {
				fmt.Fprintf(conn, `{"ok":false,"error":%q}`+"\n", err.Error())
				continue
			} else if steered {
				s.noteSteer(cmd.Message)
				fmt.Fprintln(conn, `{"ok":true,"steered":true}`)
				continue
			}
		}

		// No live agent: this command STARTS a turn, which can take many minutes.
		// Ack that it was accepted and run it in the background — the caller asked us
		// to do a thing, not to hold its connection open while an LLM thinks.
		//
		// The outcome lands in state.json (status / steering / steer_why_not), which
		// is where `foreman status` reads it from, and is the honest place for it: a
		// 3-second ack could never have carried the result of a ten-minute turn.
		go func(cmd Command) {
			if err := s.Apply(ctx, cmd); err != nil {
				_ = s.store.SaveState(s.State())
				return
			}
			_ = s.store.SaveState(s.State())
		}(cmd)
		fmt.Fprintln(conn, `{"ok":true,"accepted":true}`)
		continue
	}
	_ = sc.Err()
}

func SendCommand(root, id string, cmd Command) error {
	store := NewStore(root, id)
	var ack struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := agentlaunch.SendJSONControl(store.CtlSockPath(), cmd, &ack, 3*time.Second); err != nil {
		return err
	}
	if !ack.OK {
		return fmt.Errorf("foreman: control command failed: %s", ack.Error)
	}
	return nil
}

func Tell(root, id, msg string) error {
	return SendCommand(root, id, Command{Verb: CommandTell, Message: msg})
}
