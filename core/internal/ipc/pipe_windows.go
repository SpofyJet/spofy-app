package ipc

import (
	"net"

	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wireguard/ipc/namedpipe"
)

const PipeAccess = "D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;0x12008b;;;IU)"

func ListenPipe(path string) (net.Listener, error) {
	access := PipeAccess
	if system() {
		access = "O:SY" + access
	}
	sd, err := windows.SecurityDescriptorFromString(access)
	if err != nil {
		return nil, err
	}
	return (&namedpipe.ListenConfig{SecurityDescriptor: sd, InputBufferSize: 64 << 10, OutputBufferSize: sendBuffer}).Listen(path)
}

func system() bool {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	return err == nil && user.User.Sid.IsWellKnown(windows.WinLocalSystemSid)
}
