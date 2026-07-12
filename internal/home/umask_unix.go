//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package home

import "syscall"

func SetRestrictiveUmask() {
	syscall.Umask(0o077)
}
