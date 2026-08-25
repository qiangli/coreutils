//go:build !unix

package paxcmd

import "os"

func defaultLinkSource(source, target string) error { return os.Link(source, target) }
