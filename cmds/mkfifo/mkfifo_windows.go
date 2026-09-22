//go:build windows

package mkfifocmd

import (
	"os"

	"golang.org/x/sys/windows"
)

// makeFIFO creates the durable half of a Windows FIFO: the marker file of
// docs/windows-fifo.md, carrying the \\.\pipe\ leaf that openers rendezvous
// on. No named pipe is created here — a pipe instance dies with its creating
// process while a FIFO must outlive mkfifo, so instances materialize at open
// time (readers are servers, writers are clients; see the document).
//
// The mode operand is applied by run's Chmod exactly as for every other
// created file on this platform; like mkdir and touch, mkfifo does not write
// the recorded ACL mode itself — chmod is that scheme's one writer.
func makeFIFO(path string, _ uint32) error {
	leaf, err := NewPipeLeaf()
	if err != nil {
		return err
	}
	// O_EXCL is mkfifo(3)'s EEXIST contract; 0o666 mirrors the a=rw default
	// (this platform derives its io/fs perms from attributes regardless).
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
	if err != nil {
		return err
	}
	_, werr := f.Write(EncodeMarker(leaf))
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr == nil {
		// Detection requires FILE_ATTRIBUTE_SYSTEM (Cygwin's convention for
		// its special files): magic bytes alone must not turn a user's
		// ordinary file into a FIFO.
		werr = setSystemAttribute(path)
	}
	if werr != nil {
		// A marker that will never be detected is worse than no file at all.
		os.Remove(path)
		return werr
	}
	return nil
}

func setSystemAttribute(path string) error {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	attrs, err := windows.GetFileAttributes(p)
	if err != nil {
		return err
	}
	return windows.SetFileAttributes(p, attrs|windows.FILE_ATTRIBUTE_SYSTEM)
}
