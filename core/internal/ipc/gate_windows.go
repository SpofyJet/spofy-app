package ipc

import (
	"net"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

type gate struct {
	net.Listener
	heard func(pipe windows.Handle) bool
}

func (g gate) Accept() (net.Conn, error) {
	for {
		c, err := g.Listener.Accept()
		if err != nil {
			return nil, err
		}
		if p, ok := c.(interface{ Handle() windows.Handle }); ok && g.heard(p.Handle()) {
			return c, nil
		}
		c.Close()
	}
}

func ours() ([]string, error) {
	self, err := image(windows.CurrentProcess())
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(self)
	return []string{filepath.Join(dir, "spofy.exe"), filepath.Join(dir, "spofy-tray.exe")}, nil
}

func heard(programs []string) func(windows.Handle) bool {
	return func(pipe windows.Handle) bool {
		var pid uint32
		if windows.GetNamedPipeClientProcessId(pipe, &pid) != nil {
			return false
		}
		p, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
		if err != nil {
			return false
		}
		defer windows.CloseHandle(p)
		if empowered(p) {
			return true
		}
		name, err := image(p)
		if err != nil {
			return false
		}
		for _, program := range programs {
			if strings.EqualFold(name, program) {
				return true
			}
		}
		return false
	}
}

func empowered(p windows.Handle) bool {
	var primary windows.Token
	if windows.OpenProcessToken(p, windows.TOKEN_QUERY|windows.TOKEN_DUPLICATE, &primary) != nil {
		return false
	}
	defer primary.Close()
	if user, err := primary.GetTokenUser(); err == nil && user.User.Sid.IsWellKnown(windows.WinLocalSystemSid) {
		return true
	}
	var check windows.Token
	if windows.DuplicateTokenEx(primary, windows.TOKEN_QUERY, nil, windows.SecurityIdentification, windows.TokenImpersonation, &check) != nil {
		return false
	}
	defer check.Close()
	admins, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return false
	}
	member, err := check.IsMember(admins)
	return err == nil && member
}

func image(p windows.Handle) (string, error) {
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(p, 0, &buf[0], &n); err != nil {
		return "", err
	}
	return windows.UTF16ToString(buf[:n]), nil
}
