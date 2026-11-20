package ipc

import (
	"net"

	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wireguard/ipc/namedpipe"
)

const PipeAccess = "D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;0x12008b;;;IU)"

func ListenPipe(path string) (net.Listener, error) {
	access, served := PipeAccess, system()
	if served {
		access = "O:SY" + access
	}
	sd, err := windows.SecurityDescriptorFromString(access)
	if err != nil {
		return nil, err
	}
	l, err := (&namedpipe.ListenConfig{SecurityDescriptor: sd, InputBufferSize: 64 << 10, OutputBufferSize: sendBuffer}).Listen(path)
	if err != nil || !served {
		return l, err
	}
	programs, err := ours()
	if err != nil {
		l.Close()
		return nil, err
	}
	return gate{Listener: l, heard: heard(programs)}, nil
}

func system() bool {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	return err == nil && user.User.Sid.IsWellKnown(windows.WinLocalSystemSid)
}
