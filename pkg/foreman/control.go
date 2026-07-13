package foreman

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
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
		s.handleControlConn(ctx, conn)
		if s.state.Stopped {
			return nil
		}
	}
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
		if err := s.Apply(ctx, cmd); err != nil {
			_ = s.store.SaveState(s.State())
			fmt.Fprintf(conn, `{"ok":false,"error":%q}`+"\n", err.Error())
			continue
		}
		if err := s.store.SaveState(s.State()); err != nil {
			fmt.Fprintf(conn, `{"ok":false,"error":%q}`+"\n", err.Error())
			continue
		}
		fmt.Fprintln(conn, `{"ok":true}`)
	}
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
