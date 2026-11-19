package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/SpofyJet/spofy-app/core/internal/engine"
	"github.com/SpofyJet/spofy-app/core/internal/sub"
	"github.com/SpofyJet/spofy-app/core/internal/supervisor"
	"github.com/SpofyJet/spofy-app/core/internal/xcfg"
)

var version = "dev"

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: spofy-core engine | run | version")
		os.Exit(2)
	}
	switch os.Args[1] {
	case "engine":
		ctl := controlChannel()
		if err := engine.Serve(os.Stdin, ctl); err != nil {
			os.Exit(1)
		}
	case "run":
		os.Exit(run(os.Args[2:]))
	case "version":
		fmt.Println(version)
	default:
		os.Exit(2)
	}
}

func say(v map[string]any) {
	v["t"] = time.Now().UnixMilli()
	b, _ := json.Marshal(v)
	fmt.Println(string(b))
}

func run(args []string) int {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	file := fs.String("sub", "", "subscription json file")
	pick := fs.String("server", "", "server id or part of its name")
	mode := fs.String("mode", "tun", "tun or proxy")
	level := fs.String("log", "warning", "core log level")
	port := fs.Int("port", 2080, "proxy mode port")
	guarded := fs.Bool("guard", false, "block traffic outside the tunnel while connected")
	marked := fs.Bool("lab-identity", false, "run the engine under the lab group id")
	fs.Parse(args)

	body, err := os.ReadFile(*file)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	profile, err := sub.Parse(body, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	var server *sub.Server
	for i, s := range profile.Servers {
		if s.ID == *pick || strings.Contains(s.Name, *pick) {
			server = &profile.Servers[i]
			break
		}
	}
	if server == nil {
		fmt.Fprintln(os.Stderr, "server not found")
		return 1
	}
	opts := xcfg.Options{Mode: xcfg.ModeTun, LogLevel: *level, Tun: xcfg.Tun{
		Name: "spofy0", MTU: 1500, Gateway: []string{"172.19.0.1/30"}, Routes: []string{"0.0.0.0/1", "128.0.0.0/1"},
	}}
	tag := xcfg.TagTun
	if *mode == "proxy" {
		opts.Mode, tag = xcfg.ModeProxy, xcfg.TagProxy
		opts.Proxy = xcfg.Proxy{Port: *port, User: "spofy", Pass: "spofy"}
	}
	cfg, err := xcfg.Build(server.Config, opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	var g supervisor.Guard
	if *guarded && opts.Mode == xcfg.ModeTun {
		g = labGuard(opts.Tun.Name)
	}
	sv := supervisor.New(supervisor.Options{
		Guard: g,
		Command: func() *exec.Cmd {
			cmd := exec.Command(exe, "engine")
			if *marked || *guarded {
				labIdentity(cmd)
			}
			return cmd
		},
		OnEvent: func(e supervisor.Event) {
			if e.Kind == supervisor.KindLog {
				fmt.Fprintln(os.Stderr, e.Line)
				return
			}
			say(map[string]any{"event": e.Kind, "downtimeMs": e.Downtime.Milliseconds(), "code": e.Code, "error": e.Error})
		},
	})
	defer sv.Close()
	took, err := sv.Start(cfg)
	if err != nil {
		say(map[string]any{"event": "failed", "error": err.Error()})
		return 1
	}
	say(map[string]any{"event": "started", "ms": took.Milliseconds(), "server": server.ID, "pid": sv.Pid()})

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, append([]os.Signal{syscall.SIGINT, syscall.SIGTERM}, statSignals...)...)
	for s := range sig {
		if s != syscall.SIGINT && s != syscall.SIGTERM {
			tr, _ := sv.Traffic(tag)
			say(map[string]any{"event": "traffic", "up": tr.Up, "down": tr.Down, "pid": sv.Pid()})
			continue
		}
		break
	}
	sv.Stop()
	say(map[string]any{"event": "stopped"})
	return 0
}
