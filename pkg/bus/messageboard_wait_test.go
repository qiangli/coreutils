package bus

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func runMessageBoard(t *testing.T, ctx context.Context, args ...string) (string, string, error) {
	t.Helper()
	cmd := NewMessageBoardCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	cmd.SetContext(ctx)
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

func TestMessageBoardWaitReturnsOnRelevantPost(t *testing.T) {
	boardInTempHome(t)
	posted := make(chan error, 1)
	go func() {
		time.Sleep(25 * time.Millisecond)
		posted <- PostMessage(Post{From: "profile-c", To: "profile-b", Body: "shared gate changed"})
	}()

	out, _, err := runMessageBoard(t, context.Background(), "--as", "profile-b", "--wait", "2s")
	if err != nil {
		t.Fatal(err)
	}
	if err := <-posted; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "shared gate changed") {
		t.Fatalf("wait returned without the new post:\n%s", out)
	}
	if directed, other, _, err := Unseen("profile-b", 0); err != nil || len(directed)+len(other) != 0 {
		t.Fatalf("waited read did not advance the cursor: directed=%d other=%d err=%v", len(directed), len(other), err)
	}
}

func TestMessageBoardWaitReturnsExistingPostImmediately(t *testing.T) {
	boardInTempHome(t)
	if err := PostMessage(Post{From: "profile-c", To: "profile-b", Body: "already waiting"}); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	out, _, err := runMessageBoard(t, context.Background(), "--as", "profile-b", "--wait", "2s")
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("existing post waited %s instead of returning immediately", elapsed)
	}
	if !strings.Contains(out, "already waiting") {
		t.Fatalf("existing post was not rendered:\n%s", out)
	}
}

func TestMessageBoardWaitTimeoutIsAnEmptySuccessfulRead(t *testing.T) {
	boardInTempHome(t)
	start := time.Now()
	out, errOut, err := runMessageBoard(t, context.Background(), "--as", "profile-b", "--wait", "40ms")
	if err != nil {
		t.Fatal(err)
	}
	if out != "" {
		t.Fatalf("timeout wrote board output %q", out)
	}
	if !strings.Contains(errOut, "nothing new on the board") {
		t.Fatalf("timeout did not report the empty read: %q", errOut)
	}
	if elapsed := time.Since(start); elapsed < 30*time.Millisecond {
		t.Fatalf("wait returned before its bound: %s", elapsed)
	}
}

func TestMessageBoardWaitHonorsCancellation(t *testing.T) {
	boardInTempHome(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := runMessageBoard(t, ctx, "--as", "profile-b", "--wait", "2s")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled wait returned %v", err)
	}
}

func TestMessageBoardWaitRejectsWholeHistory(t *testing.T) {
	boardInTempHome(t)
	_, _, err := runMessageBoard(t, context.Background(), "--as", "profile-b", "--history", "--wait", "1s")
	if err == nil || !strings.Contains(err.Error(), "cannot be combined with --history") {
		t.Fatalf("--history --wait returned %v", err)
	}
}

func TestMessageBoardAllIsHiddenDeprecatedHistoryAlias(t *testing.T) {
	cmd := NewMessageBoardCmd()
	flag := cmd.Flags().Lookup("all")
	if flag == nil || !flag.Hidden {
		t.Fatalf("--all alias must exist and be hidden, got %#v", flag)
	}

	boardInTempHome(t)
	if err := PostMessage(Post{From: "sender", Body: "old post"}); err != nil {
		t.Fatal(err)
	}
	out, errOut, err := runMessageBoard(t, context.Background(), "--as", "reader", "--all")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "old post") {
		t.Fatalf("--all alias did not show history:\n%s", out)
	}
	if !strings.Contains(errOut, "--all is deprecated; use --history") {
		t.Fatalf("--all alias did not print replacement notice: %q", errOut)
	}
}
