//go:build !linux

package nproccmd

func cgroupQuota() int { return 0 }
