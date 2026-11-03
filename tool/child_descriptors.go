package tool

import "os/exec"

// StartOwnedCommand adds the invocation's explicitly owned descriptors to a
// child. Preparation never changes argv or environment. The child owns its
// copies after Start returns, so parent-only duplicates are released on both
// successful and failed starts, before a caller waits for EOF.
func (rc *RunContext) StartOwnedCommand(c *exec.Cmd) error {
	return rc.startCommandDescriptors(c, StartOwnedCommand)
}

// RunOwnedCommand preserves the ordinary wait and exit-status contract after
// the invocation-owned descriptor handoff.
func (rc *RunContext) RunOwnedCommand(c *exec.Cmd) error {
	if err := rc.StartOwnedCommand(c); err != nil {
		return err
	}
	return c.Wait()
}

// prepareCommandDescriptors normalizes the optional descriptor-only adapter.
// Call cleanup after Start (or a failed preparation); it releases parent copies,
// never child-owned handles or asynchronous stream bridges.
func (rc *RunContext) prepareCommandDescriptors(c *exec.Cmd) (func(), error) {
	if rc.PrepareChildDescriptors == nil {
		return func() {}, nil
	}
	cleanup, err := rc.PrepareChildDescriptors(c)
	if cleanup == nil {
		cleanup = func() {}
	}
	return cleanup, err
}

// RunCommandWithDescriptors preserves exec.Cmd.Run semantics for callers which
// deliberately do not use the owned-image framing path.
func (rc *RunContext) RunCommandWithDescriptors(c *exec.Cmd) error {
	if err := rc.startCommandDescriptors(c, (*exec.Cmd).Start); err != nil {
		return err
	}
	return c.Wait()
}

func (rc *RunContext) startCommandDescriptors(c *exec.Cmd, start func(*exec.Cmd) error) error {
	cleanup, err := rc.prepareCommandDescriptors(c)
	defer cleanup()
	if err != nil {
		return err
	}
	return start(c)
}
