package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/SpofyJet/spofy-app/core/internal/engine"
	"github.com/SpofyJet/spofy-app/core/internal/session"
	"github.com/SpofyJet/spofy-app/core/internal/sub"
	"github.com/SpofyJet/spofy-app/core/internal/supervisor"
	"github.com/SpofyJet/spofy-app/core/internal/xcfg"
)

var version = "dev"

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: spofy-core engine | run | measure | version")
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
	case "measure":
		os.Exit(measure(os.Args[2:]))
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

func load(file string) (*sub.Profile, error) {
	body, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	return sub.Parse(body, nil)
}

func command(lab bool) (func() *exec.Cmd, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	return func() *exec.Cmd {
		cmd := exec.Command(exe, "engine")
		if lab {
			labIdentity(cmd)
		}
		return cmd
	}, nil
}

func candidates(p *sub.Profile, o xcfg.Options) ([]session.Candidate, error) {
	var out []session.Candidate
	for _, s := range p.Servers {
		run, err := xcfg.Build(s.Config, o)
		if err != nil {
			return nil, err
		}
		probe, err := xcfg.Build(s.Config, xcfg.Options{Mode: xcfg.ModeProbe, LogLevel: "none"})
		if err != nil {
			return nil, err
		}
		out = append(out, session.Candidate{ID: s.ID, Run: run, Probe: probe, Tag: xcfg.MainTag(s.Config)})
	}
	return out, nil
}

func measure(args []string) int {
	fs := flag.NewFlagSet("measure", flag.ExitOnError)
	file := fs.String("sub", "", "subscription json file")
	parallel := fs.Int("parallel", 4, "prober processes")
	timeout := fs.Duration("timeout", 5*time.Second, "limit for one server")
	url := fs.String("url", "", "probe address")
	marked := fs.Bool("lab-identity", false, "run probers under the lab group id")
	fs.Parse(args)

	profile, err := load(*file)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	cands, err := candidates(profile, xcfg.Options{Mode: xcfg.ModeProbe})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	cmd, err := command(*marked)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	sv := supervisor.New(supervisor.Options{Command: cmd})
	defer sv.Close()
	ss := session.New(session.Options{Tunnel: sv, URL: *url, Timeout: *timeout, Parallel: *parallel})
	began := time.Now()
	ok := 0
	for i, m := range ss.Measure(context.Background(), cands) {
		if m.OK() {
			ok++
		}
		say(map[string]any{"server": cands[i].ID, "ms": m.Delay.Milliseconds(), "code": m.Code, "error": m.Error})
	}
	say(map[string]any{"event": "measured", "servers": len(cands), "ok": ok, "ms": time.Since(began).Milliseconds()})
	return 0
}

func run(args []string) int {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	file := fs.String("sub", "", "subscription json file")
	pick := fs.String("server", "auto", "auto, server id or part of its name")
	mode := fs.String("mode", "tun", "tun or proxy")
	level := fs.String("log", "warning", "core log level")
	port := fs.Int("port", 2080, "proxy mode port")
	guarded := fs.Bool("guard", false, "block traffic outside the tunnel while connected")
	strict := fs.Bool("strict", false, "keep the block when the first connection fails")
	marked := fs.Bool("lab-identity", false, "run the engine under the lab group id")
	every := fs.Duration("check", 0, "probe the tunnel at this interval")
	timeout := fs.Duration("timeout", 5*time.Second, "limit for one probe")
	url := fs.String("url", "", "probe address")
	fs.Parse(args)

	profile, err := load(*file)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	pinned := ""
	if *pick != "auto" {
		for _, s := range profile.Servers {
			if s.ID == *pick || strings.Contains(s.Name, *pick) {
				pinned = s.ID
				break
			}
		}
		if pinned == "" {
			fmt.Fprintln(os.Stderr, "server not found")
			return 1
		}
	}
	opts := xcfg.Options{Mode: xcfg.ModeTun, LogLevel: *level, Tun: xcfg.Tun{
		Name: "spofy0", MTU: 1500, Gateway: []string{"172.19.0.1/30"}, Routes: []string{"0.0.0.0/1", "128.0.0.0/1"},
	}}
	tag := xcfg.TagTun
	if *mode == "proxy" {
		opts.Mode, tag = xcfg.ModeProxy, xcfg.TagProxy
		opts.Proxy = xcfg.Proxy{Port: *port, User: "spofy", Pass: "spofy"}
	}
	cands, err := candidates(profile, opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	cmd, err := command(*marked || *guarded)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	var g supervisor.Guard
	if *guarded && opts.Mode == xcfg.ModeTun {
		g = labGuard(opts.Tun.Name)
	}
	var ss *session.Session
	var checks sync.WaitGroup
	check := func() {
		checks.Add(1)
		go func() {
			defer checks.Done()
			ss.Check(context.Background())
		}()
	}
	sv := supervisor.New(supervisor.Options{
		Guard:     g,
		HoldGuard: true,
		Command:   cmd,
		OnEvent: func(e supervisor.Event) {
			if e.Kind == supervisor.KindLog {
				fmt.Fprintln(os.Stderr, e.Line)
				return
			}
			say(map[string]any{"event": e.Kind, "downtimeMs": e.Downtime.Milliseconds(), "code": e.Code, "error": e.Error})
			if e.Kind == supervisor.KindRestored || e.Kind == supervisor.KindFailed {
				check()
			}
		},
	})
	defer sv.Close()
	ss = session.New(session.Options{
		Tunnel: sv, Strict: *strict, URL: *url, Timeout: *timeout,
		OnEvent: func(e session.Event) {
			say(map[string]any{"event": "state", "state": e.State, "server": e.Server, "ms": e.Delay.Milliseconds(), "code": e.Code, "error": e.Error, "pid": sv.Pid()})
		},
	})
	began := time.Now()
	if err := ss.Connect(context.Background(), cands, pinned); err != nil {
		say(map[string]any{"event": "failed", "error": err.Error()})
		return 1
	}
	say(map[string]any{"event": "started", "ms": time.Since(began).Milliseconds(), "pid": sv.Pid()})

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, append([]os.Signal{syscall.SIGINT, syscall.SIGTERM}, statSignals...)...)
	var tick <-chan time.Time
	if *every > 0 {
		t := time.NewTicker(*every)
		defer t.Stop()
		tick = t.C
	}
	for {
		select {
		case <-tick:
			check()
			continue
		case s := <-sig:
			if s != syscall.SIGINT && s != syscall.SIGTERM {
				tr, _ := sv.Traffic(tag)
				say(map[string]any{"event": "traffic", "up": tr.Up, "down": tr.Down, "pid": sv.Pid()})
				continue
			}
		}
		break
	}
	ss.Disconnect()
	checks.Wait()
	say(map[string]any{"event": "stopped"})
	return 0
}
