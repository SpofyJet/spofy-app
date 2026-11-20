package ipc

import (
	"net"

	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wireguard/ipc/namedpipe"
)

const PipeAccess = "D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;0x12008b;;;IU)"

func ListenPipe(path string) (net.Listener, error) {
	sd, err := windows.SecurityDescriptorFromString(PipeAccess)
	if err != nil {
		return nil, err
	}
	return (&namedpipe.ListenConfig{SecurityDescriptor: sd, InputBufferSize: 64 << 10, OutputBufferSize: sendBuffer}).Listen(path)
}
