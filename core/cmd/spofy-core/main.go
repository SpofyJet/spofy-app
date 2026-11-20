package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/SpofyJet/spofy-app/core/internal/app"
	"github.com/SpofyJet/spofy-app/core/internal/engine"
	"github.com/SpofyJet/spofy-app/core/internal/host"
	"github.com/SpofyJet/spofy-app/core/internal/ipc"
	"github.com/SpofyJet/spofy-app/core/internal/session"
	"github.com/SpofyJet/spofy-app/core/internal/store"
	"github.com/SpofyJet/spofy-app/core/internal/sub"
	"github.com/SpofyJet/spofy-app/core/internal/supervisor"
	"github.com/SpofyJet/spofy-app/core/internal/xcfg"
)

var version = "dev"

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: spofy-core engine | serve | run | measure | version")
		os.Exit(2)
	}
	switch os.Args[1] {
	case "engine":
		ctl := controlChannel()
		if err := engine.Serve(host.Stdin(), ctl); err != nil {
			os.Exit(1)
		}
	case "serve":
		os.Exit(serve(os.Args[2:]))
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

func state(dir string) *store.Store {
	if dir == "" {
		return nil
	}
	key, err := base64.StdEncoding.DecodeString(os.Getenv("SPOFY_STATE_KEY"))
	if err != nil {
		key = nil
	}
	st, err := store.Open(dir, key)
	if err != nil {
		say(map[string]any{"event": "state_unavailable", "error": err.Error(), "unreadable": errors.Is(err, store.ErrUnreadable)})
		return nil
	}
	say(map[string]any{"event": "state_loaded", "known": len(st.Get().Delays), "recovered": st.Recovered()})
	return st
}

func recall(st *store.Store, cands []session.Candidate, fresh time.Duration) {
	if st == nil {
		return
	}
	delays := st.Get().Delays
	for i, c := range cands {
		if d, ok := delays[c.ID]; ok && d.Ms > 0 && time.Since(d.At) < fresh {
			cands[i].Known = time.Duration(d.Ms) * time.Millisecond
		}
	}
}

func remember(st *store.Store) func([]session.Candidate, []supervisor.Measurement) {
	if st == nil {
		return nil
	}
	return func(cands []session.Candidate, measured []supervisor.Measurement) {
		err := st.Update(func(s *store.State) {
			s.Delays = map[string]store.Delay{}
			for i, m := range measured {
				if m.OK() {
					s.Delays[cands[i].ID] = store.Delay{Ms: max(m.Delay.Milliseconds(), 1), At: time.Now().UTC().Truncate(time.Second)}
				}
			}
		})
		if err != nil {
			say(map[string]any{"event": "state_unavailable", "error": err.Error()})
		}
	}
}

const resolver = "172.19.0.2"

func tunnel(mode string, port int, level string) (xcfg.Options, error) {
	opts := xcfg.Options{Mode: xcfg.ModeTun, LogLevel: level, Tun: xcfg.Tun{
		Name: "spofy0", MTU: 1500, Gateway: []string{"172.19.0.1/30"}, Routes: []string{"0.0.0.0/1", "128.0.0.0/1"},
	}}
	switch mode {
	case "tun":
	case "proxy":
		opts.Mode = xcfg.ModeProxy
		opts.Proxy = xcfg.Proxy{Port: port, User: "spofy", Pass: "spofy"}
	default:
		return opts, errors.New("mode must be tun or proxy")
	}
	return opts, nil
}

func serve(args []string) int {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	folder := fs.String("state", "", "folder for saved state, the key comes from SPOFY_STATE_KEY")
	mode := fs.String("mode", "tun", "tun or proxy")
	level := fs.String("log", "warning", "core log level")
	port := fs.Int("port", 2080, "proxy mode port")
	guarded := fs.Bool("guard", false, "block traffic outside the tunnel while connected")
	strict := fs.Bool("strict", false, "keep the block when the first connection fails")
	marked := fs.Bool("lab-identity", false, "run the engine under the lab group id")
	hwid := fs.String("hwid", "", "device id sent to the subscription server")
	system := fs.String("os", "Linux", "system name sent to the subscription server")
	release := fs.String("os-version", "lab", "system version sent to the subscription server")
	model := fs.String("model", "lab", "device model sent to the subscription server")
	socket := fs.String("listen", "", "serve the channel on this unix socket, the core stops when its input closes")
	given := fs.Bool("platform", false, "take the tunnel from the platform, needs -listen")
	bypass := fs.Bool("bypass", false, "the core's own connections go outside the tunnel: servers are measured while it is up")
	watch := fs.Duration("watch", time.Second, "how often the traffic counters are read while connected, 0 turns the watch off")
	quiet := fs.Duration("quiet", 5*time.Second, "check the connection when traffic goes out for this long and nothing comes back")
	renew := fs.Duration("renew", 15*time.Minute, "the subscription is fetched again no more often than this")
	fs.Parse(args)

	out := controlChannel()
	fail := func(err error) int {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	key, err := base64.StdEncoding.DecodeString(os.Getenv("SPOFY_STATE_KEY"))
	if err != nil || *folder == "" {
		return fail(errors.New("serve needs -state and SPOFY_STATE_KEY"))
	}
	st, err := store.Open(*folder, key)
	if err != nil {
		return fail(err)
	}
	opts, err := tunnel(*mode, *port, *level)
	if err != nil {
		return fail(err)
	}
	cmd, err := command(*marked || *guarded)
	if err != nil {
		return fail(err)
	}
	var g supervisor.Guard
	if *guarded && opts.Mode == xcfg.ModeTun {
		g = labGuard(opts.Tun.Name)
	}
	var platform *host.Platform
	var carried func() *os.File
	var retune func() (bool, error)
	var a *app.App
	if *given {
		if *socket == "" || opts.Mode != xcfg.ModeTun {
			return fail(errors.New("-platform needs -listen and the tun mode"))
		}
		platform = &host.Platform{}
		t := &host.Tun{Platform: platform, Spec: host.Spec{
			Name: opts.Tun.Name, MTU: opts.Tun.MTU, Addresses: []string{"172.19.0.1/30", "fdfe:5f0f::1/126"}, Routes: []string{"0.0.0.0/0", "::/0"}, DNS: []string{resolver},
		}}
		t.Apps = func() *host.Apps {
			if chosen := a.Apps(); chosen != nil {
				return &host.Apps{Mode: chosen.Mode, List: chosen.List}
			}
			return nil
		}
		g, carried, retune = t, t.File, t.Renew
		opts.Tun = xcfg.Tun{Name: opts.Tun.Name, MTU: opts.Tun.MTU, Resolver: []string{resolver}}
	}
	var checks sync.WaitGroup
	sv := supervisor.New(supervisor.Options{
		Guard:     g,
		HoldGuard: true,
		Tunnel:    carried,
		Command:   cmd,
		OnEvent: func(e supervisor.Event) {
			switch e.Kind {
			case supervisor.KindLog:
				fmt.Fprintln(os.Stderr, e.Line)
			case supervisor.KindRestored, supervisor.KindFailed:
				checks.Add(1)
				go func() {
					defer checks.Done()
					a.Check()
				}()
			}
		},
	})
	defer sv.Close()
	a = app.New(app.Config{
		Store:   st,
		Tunnel:  sv,
		Device:  sub.Device{HWID: *hwid, OS: *system, OSVersion: *release, Model: *model, AppVersion: version},
		Run:     opts,
		Version: version,
		Strict:  *strict,
		Bypass:  *bypass,
		Watch:   *watch,
		Quiet:   *quiet,
		Renew:   *renew,
		Retune:  retune,
	})
	checks.Add(1)
	go func() {
		defer checks.Done()
		a.Resume()
	}()
	if *socket != "" {
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			io.Copy(io.Discard, os.Stdin)
			cancel()
		}()
		err = listen(ctx, *socket, a, platform, func() { fmt.Fprintln(out, `{"ev":"listening"}`) })
		cancel()
	} else {
		err = ipc.Serve(context.Background(), os.Stdin, out, a, nil)
	}
	a.Close()
	checks.Wait()
	if err != nil {
		return fail(err)
	}
	return 0
}

func measure(args []string) int {
	fs := flag.NewFlagSet("measure", flag.ExitOnError)
	file := fs.String("sub", "", "subscription json file")
	parallel := fs.Int("parallel", 4, "prober processes")
	timeout := fs.Duration("timeout", 5*time.Second, "limit for one server")
	url := fs.String("url", "", "probe address")
	marked := fs.Bool("lab-identity", false, "run probers under the lab group id")
	folder := fs.String("state", "", "folder for saved state, the key comes from SPOFY_STATE_KEY")
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
	measured := ss.Measure(context.Background(), cands)
	if save := remember(state(*folder)); save != nil {
		save(cands, measured)
	}
	for i, m := range measured {
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
	folder := fs.String("state", "", "folder for saved state, the key comes from SPOFY_STATE_KEY")
	fresh := fs.Duration("fresh", 6*time.Hour, "how long a saved measurement is trusted")
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
	opts, err := tunnel(*mode, *port, *level)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	tag := xcfg.TagTun
	if opts.Mode == xcfg.ModeProxy {
		tag = xcfg.TagProxy
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
	st := state(*folder)
	recall(st, cands, *fresh)
	ss = session.New(session.Options{
		Tunnel: sv, Strict: *strict, URL: *url, Timeout: *timeout, OnMeasure: remember(st),
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
