package guard

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"
)

const EngineGID = 4711

const table = "spofy_guard"

type Linux struct {
	Tun string

	mu  sync.Mutex
	cmd *exec.Cmd
	in  io.WriteCloser
}

func (g *Linux) Raise() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.cmd != nil {
		return nil
	}
	cmd := exec.Command("nft", "-i")
	in, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = io.Discard, &stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	g.cmd, g.in = cmd, in
	rules := fmt.Sprintf(`add table inet %[1]s { flags owner; }
add chain inet %[1]s out { type filter hook output priority 0; policy drop; }
add rule inet %[1]s out oifname "lo" accept
add rule inet %[1]s out oifname "%[2]s" accept
add rule inet %[1]s out meta skgid %[3]d accept
add rule inet %[1]s out counter drop
`, table, g.Tun, EngineGID)
	if _, err := io.WriteString(in, rules); err != nil {
		g.lower()
		return err
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		out, _ := exec.Command("nft", "list", "chain", "inet", table, "out").Output()
		if bytes.Contains(out, []byte("policy drop")) && bytes.Contains(out, []byte("counter packets")) {
			return nil
		}
		if time.Now().After(deadline) {
			g.lower()
			return errors.New("guard rules were not applied: " + stderr.String())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (g *Linux) Lower() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.lower()
	return nil
}

func (g *Linux) lower() {
	if g.cmd == nil {
		return
	}
	g.in.Close()
	g.cmd.Wait()
	g.cmd, g.in = nil, nil
}
