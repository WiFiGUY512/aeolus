//go:build unix

package mdns

import (
	"errors"
	"net"
	"syscall"
)

// joinGroup has the socket hear the group on the network this host's
// address ifaddr is on. One socket hears it on every network joined, so a
// question is answered once.
func joinGroup(conn *net.UDPConn, group, ifaddr net.IP) error {
	raw, err := conn.SyscallConn()
	if err != nil {
		return err
	}
	var mreq syscall.IPMreq
	copy(mreq.Multiaddr[:], group.To4())
	copy(mreq.Interface[:], ifaddr.To4())
	var serr error
	if err := raw.Control(func(fd uintptr) {
		serr = syscall.SetsockoptIPMreq(int(fd), syscall.IPPROTO_IP, syscall.IP_ADD_MEMBERSHIP, &mreq)
	}); err != nil {
		return err
	}
	// Joined already, as the network the socket was opened on is.
	if errors.Is(serr, syscall.EADDRINUSE) {
		return nil
	}
	return serr
}

// sendFrom has what the socket next sends to the group go out by the
// network this host's address ifaddr is on.
func sendFrom(conn *net.UDPConn, ifaddr net.IP) error {
	raw, err := conn.SyscallConn()
	if err != nil {
		return err
	}
	var a [4]byte
	copy(a[:], ifaddr.To4())
	var serr error
	if err := raw.Control(func(fd uintptr) {
		serr = syscall.SetsockoptInet4Addr(int(fd), syscall.IPPROTO_IP, syscall.IP_MULTICAST_IF, a)
	}); err != nil {
		return err
	}
	return serr
}
