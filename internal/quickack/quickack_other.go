//go:build !linux

package quickack

import "net"

// Listener is l as it is: only Linux has the setting.
func Listener(l net.Listener) net.Listener { return l }
