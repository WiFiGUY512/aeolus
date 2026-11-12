//go:build !unix

package mdns

import (
	"errors"
	"net"
)

// A manager runs on Linux; elsewhere (a developer's Windows) the socket
// hears the group on the one network it was opened on, and answers by the
// system's choice of network.
var errNotHere = errors.New("mdns: one network only on this system")

func joinGroup(conn *net.UDPConn, group, ifaddr net.IP) error { return errNotHere }

func sendFrom(conn *net.UDPConn, ifaddr net.IP) error { return errNotHere }
