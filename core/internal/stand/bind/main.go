//go:build linux

package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"syscall"
	"time"
)

func main() {
	if len(os.Args) != 5 {
		fmt.Fprintln(os.Stderr, "usage: bind <interface> <host> <port> <path>")
		os.Exit(2)
	}
	iface, host, port, path := os.Args[1], os.Args[2], os.Args[3], os.Args[4]
	d := net.Dialer{Timeout: 5 * time.Second, Control: func(_, _ string, c syscall.RawConn) error {
		var err error
		if e := c.Control(func(fd uintptr) {
			err = syscall.SetsockoptString(int(fd), syscall.SOL_SOCKET, syscall.SO_BINDTODEVICE, iface)
		}); e != nil {
			return e
		}
		return err
	}}
	c, err := d.Dial("tcp4", net.JoinHostPort(host, port))
	if err != nil {
		fmt.Println("no connection:", err)
		return
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(8 * time.Second))
	fmt.Fprintf(c, "GET %s HTTP/1.0\r\nHost: %s\r\n\r\n", path, host)
	body, _ := io.ReadAll(c)
	fmt.Print(string(body))
}
