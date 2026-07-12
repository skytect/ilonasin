//go:build !(aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris)

package home

func SetRestrictiveUmask() {}
