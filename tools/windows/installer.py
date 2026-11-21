import ctypes
import faulthandler
import glob
import hashlib
import http.server
import os
import shutil
import ssl
import subprocess
import sys
import threading
import time
import winreg

import stand
from stand import PIPE, Pipe, expect, failed, past, pipe_open, ps, until

PACK, STUB, BUNDLE, OUT = (os.path.abspath(a) for a in sys.argv[2:6])
VERSION = sys.argv[6]
SKIP = set(os.environ.get("STAND_SKIP", "").split(","))
RELEASE = os.path.abspath(sys.argv[7])
SERVICE = r"SYSTEM\CurrentControlSet\Services\Spofy"
HOME = os.path.join(os.environ["ProgramFiles"], "Spofy")
DATA = os.path.join(os.environ["ProgramData"], "Spofy")
SHORTCUT = os.path.join(os.environ["ProgramData"], r"Microsoft\Windows\Start Menu\Programs\Spofy.lnk")
DESKTOP = os.path.join(os.environ.get("PUBLIC", r"C:\Users\Public"), "Desktop", "Spofy.lnk")
RUN = r"Software\Microsoft\Windows\CurrentVersion\Run"
ENTRY = r"Software\Microsoft\Windows\CurrentVersion\Uninstall\Spofy"
KEPT = "5f0f5a7e-1d2b-4c3a-9b61-73706f667901"
AWAY = "http://203.0.113.7/generate_204"
STANDIN = os.path.join(os.environ["SystemRoot"], "System32", "whoami.exe")


class Site(http.server.BaseHTTPRequestHandler):
    hits = 0

    def do_GET(self):
        Site.hits += 1
        self.send_response(204)
        self.end_headers()

    def log_message(self, *args):
        pass


def value(key, name):
    try:
        with winreg.OpenKey(winreg.HKEY_LOCAL_MACHINE, key, 0, winreg.KEY_READ | winreg.KEY_WOW64_64KEY) as k:
            return winreg.QueryValueEx(k, name)[0]
    except OSError:
        return None


def away(limit=3):
    r = subprocess.run(["curl.exe", "-s", "-m", str(limit), "-o", "NUL", "-w", "%{http_code}", AWAY], capture_output=True, text=True)
    return r.stdout.strip() == "204"


def wfp(kind):
    path = os.path.join(stand.WORK, kind + ".xml")
    subprocess.run(["netsh", "wfp", "show", kind, "file=" + path], capture_output=True)
    text = open(path, encoding="utf-8", errors="replace").read() if os.path.exists(path) else ""
    return KEPT in text.lower() or ">Spofy<" in text


def traces():
    return {
        "service": subprocess.run(["sc", "query", "Spofy"], capture_output=True).returncode == 0,
        "files": os.path.exists(HOME),
        "shortcut": os.path.exists(SHORTCUT),
        "desktop": os.path.exists(DESKTOP),
        "run": value(RUN, "Spofy") is not None,
        "entry": value(ENTRY, "DisplayVersion") is not None,
        "state": os.path.exists(DATA),
        "adapter": stand.adapter() != "",
        "pipe": pipe_open(),
        "sublayers": wfp("sublayers"),
        "filters": wfp("filters"),
        "driver": "wintun.inf" in subprocess.run(["pnputil", "/enum-drivers"], capture_output=True, text=True).stdout.lower(),
        "leftovers": sorted(os.path.basename(p) for p in glob.glob(HOME + ".*") + glob.glob(DATA + ".*")),
    }


def build(bundle, version):
    if not os.path.exists(os.path.join(bundle, "uninstall.exe")):
        shutil.copy(STUB, os.path.join(bundle, "uninstall.exe"))
    path = os.path.join(OUT, "Spofy-%s-x64.exe" % version)
    r = subprocess.run([PACK, "-stub", STUB, "-bundle", bundle, "-version", version, "-out", path], capture_output=True, text=True)
    if r.returncode != 0:
        raise SystemExit("pack %s: %s %s" % (version, r.stdout[-3000:], r.stderr[-3000:]))
    print("     built " + r.stdout.strip(), flush=True)
    return path


def sign(exe):
    kits = sorted(glob.glob(r"C:\Program Files (x86)\Windows Kits\10\bin\10.*\x64\signtool.exe"))
    if not kits:
        return False
    pfx = os.path.join(stand.WORK, "stand.pfx")
    if not os.path.exists(pfx):
        ps("$c = New-SelfSignedCertificate -Type CodeSigningCert -Subject 'CN=Spofy Stand' -CertStoreLocation Cert:\\CurrentUser\\My; "
           "Export-PfxCertificate -Cert $c -FilePath '%s' -Password (ConvertTo-SecureString -String 'stand' -Force -AsPlainText) | Out-Null; "
           "Remove-Item $c.PSPath" % pfx)
    r = subprocess.run([kits[-1], "sign", "/fd", "SHA256", "/f", pfx, "/p", "stand", exe], capture_output=True, text=True)
    return r.returncode == 0


def files(root):
    return {os.path.relpath(os.path.join(d, f), root).lower(): digest(os.path.join(d, f)) for d, _, fs in os.walk(root) for f in fs}


def variant(name, **swap):
    folder = os.path.join(stand.WORK, name)
    shutil.copytree(BUNDLE, folder)
    for file, source in swap.items():
        shutil.copy(source, os.path.join(folder, file.replace("_", "-") + ".exe"))
    return folder


def setup(exe):
    log = os.path.join(OUT, os.path.basename(exe) + ".log")
    at = time.time()
    code = subprocess.run([exe, "-quiet", "-log", log], timeout=600).returncode
    return code, time.time() - at


def remove():
    log = os.path.join(OUT, "uninstall.log")
    at = time.time()
    code = subprocess.run([os.path.join(HOME, "uninstall.exe"), "-uninstall", "-quiet", "-log", log], timeout=600).returncode
    gone = until(lambda: not os.path.exists(HOME) and value(ENTRY, "DisplayVersion") is None, 120, 0.1)
    return code, time.time() - at, bool(gone)


def digest(path):
    return hashlib.sha256(open(path, "rb").read()).hexdigest()


class Watch:
    def __init__(self, source):
        self.source, self.leaks, self.through, self.going = source, 0, [], True
        self.threads = [threading.Thread(target=self.leak, daemon=True), threading.Thread(target=self.tunnel, daemon=True)]
        for t in self.threads:
            t.start()

    def leak(self):
        while self.going:
            self.leaks += past(self.source, 2)

    def tunnel(self):
        while self.going:
            if away(1):
                self.through.append(time.time())
            time.sleep(0.1)

    def stop(self, began):
        self.going = False
        for t in self.threads:
            t.join(10)
        seen = [began] + [t for t in self.through if t >= began]
        return max(b - a for a, b in zip(seen, seen[1:] + [time.time()])), self.leaks


def shelf(files):
    class Handler(http.server.BaseHTTPRequestHandler):
        def do_GET(self):
            path = files.get(self.path.lstrip("/"))
            if not path:
                self.send_response(404)
                self.end_headers()
                return
            body = open(path, "rb").read()
            self.send_response(200)
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        def log_message(self, *args):
            pass
    srv = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    ctx.load_cert_chain(os.path.join(stand.WORK, "srv.pem"), os.path.join(stand.WORK, "srv.key"))
    srv.socket = ctx.wrap_socket(srv.socket, server_side=True)
    threading.Thread(target=srv.serve_forever, daemon=True).start()
    return srv, "https://127.0.0.1:%d/" % srv.server_address[1]


def release(*args):
    r = subprocess.run([RELEASE, *args], capture_output=True, text=True)
    if r.returncode != 0:
        raise SystemExit("spofy-release %s: %s %s" % (args[0], r.stdout[-1000:], r.stderr[-1000:]))
    return r.stdout.strip()


def publish(exe, version, build, base):
    key, notes, manifest, signed = (os.path.join(stand.WORK, n) for n in ("update.key", "notes.txt", "manifest.json", "update.json"))
    for path in (key, manifest, signed):
        if os.path.exists(path):
            os.remove(path)
    public = release("keygen", "-out", key)
    with open(notes, "w", encoding="utf-8") as f:
        f.write("stand")
    release("manifest", "-version", version, "-build", str(build), "-notes-file", notes, "-rollout", "100", "-url-base", base, "-file", "windows-x64=" + exe, "-out", manifest)
    release("sign", "-key", key, "-manifest", manifest, "-out", signed)
    return signed, public


def service_env(values):
    with winreg.OpenKey(winreg.HKEY_LOCAL_MACHINE, SERVICE, 0, winreg.KEY_SET_VALUE | winreg.KEY_WOW64_64KEY) as k:
        winreg.SetValueEx(k, "Environment", 0, winreg.REG_MULTI_SZ, ["%s=%s" % kv for kv in values.items()])
    for verb in ("stop", "start"):
        subprocess.run(["sc", verb, "Spofy"], capture_output=True)
        until(lambda: ("RUNNING" if verb == "start" else "STOPPED") in subprocess.run(["sc", "query", "Spofy"], capture_output=True, text=True).stdout, 60, 0.2)
    return bool(until(pipe_open, 30, 0.2))


def owner(hwnd):
    pid = ctypes.c_uint32()
    if hwnd:
        stand.user32.GetWindowThreadProcessId(hwnd, ctypes.byref(pid))
    return pid.value


def connected(limit=60):
    return bool(until(lambda: away(2), limit, 0.2))


def shut(hwnd):
    pid = owner(hwnd)
    k = ctypes.windll.kernel32
    k.OpenProcess.restype = ctypes.c_void_p
    h = k.OpenProcess(0x00100000, False, pid) if pid else None
    stand.user32.PostMessageW(hwnd, 0x10, 0, 0)
    if h:
        k.WaitForSingleObject(ctypes.c_void_p(h), 15000)
        k.CloseHandle(ctypes.c_void_p(h))


def launches(source, ui, installer):
    shots = os.path.join(OUT, "launch")
    runs = []
    print("     step: start menu launches", flush=True)
    for name in () if "launch" in SKIP else ("first", "second", "third", "idle"):
        if name == "idle":
            ui.ask("disconnect", 30)
        r = stand.launch(lambda: os.startfile(SHORTCUT), name, shots)
        runs.append(r)
        if r["hwnd"]:
            shut(r["hwnd"])
        if name == "idle":
            ui.ask("connect", 60)
    print("     step: setup again", flush=True)
    if "again" in SKIP:
        if os.path.exists(DESKTOP):
            os.remove(DESKTOP)
        return person(installer) if "person" not in SKIP else None
    clean = [r for r in runs if r["shown"] is not None and r["content"] is not None and r["frames"] and r["frames"][0][1] < 1]
    expect("opened from the Start menu, the window shows its final picture in its first frame, with no other window",
           len(clean) == len(runs) and not any(r["others"] for r in runs),
           "shown_ms=%s content_ms=%s" % ([r["shown"] for r in runs], [r["content"] for r in runs]))
    before = stand.service_pid()
    watch = Watch(source)
    began = time.time()
    started = []
    seen = stand.launch(lambda: started.append(subprocess.Popen([installer])), "setup-again", shots, limit=20.0)
    code = started[0].wait(120)
    outage, leaks = watch.stop(began)
    after = stand.service_pid()
    expect("the installer of the version that is installed opens the window with no window of its own and leaves the service and the VPN alone",
           code == 0 and seen["shown"] is not None and not seen["others"] and before == after and outage < 1.0 and leaks == 0,
           "code=%d s=%.1f shown_ms=%s front_ms=%s content_ms=%s service %d -> %d vpn_down=%.1f s leaks=%d others=%s"
           % (code, time.time() - began, seen["shown"], seen["front"], seen["content"], before, after, outage, leaks, seen["others"]))
    if seen["hwnd"]:
        shut(seen["hwnd"])
    os.remove(DESKTOP)
    if "person" not in SKIP:
        person(installer)


def person(installer):
    print("     step: setup as a person", flush=True)
    answer = []

    def run():
        try:
            answer.append(stand.as_person([installer], 60))
        except OSError as e:
            answer.append("refused %s" % e.winerror)
    before = stand.service_pid()
    shots = os.path.join(OUT, "launch")
    seen = stand.launch(lambda: threading.Thread(target=run, daemon=True).start(), "setup-person", shots, limit=10.0)
    until(lambda: answer, 60, 0.1)
    expect("a person without administrator rights opens Spofy with the installer: no request for rights, the window opens, the service stays",
           answer == [0] and seen["shown"] is not None and stand.service_pid() == before,
           "code=%s shown_ms=%s front_ms=%s content_ms=%s others=%s" % (answer, seen["shown"], seen["front"], seen["content"], seen["others"]))
    if seen["hwnd"]:
        shut(seen["hwnd"])


def scenario():
    uplink = ps("(Get-NetRoute -DestinationPrefix 0.0.0.0/0 | Sort-Object -Property RouteMetric, ifMetric | Select-Object -First 1 | Get-NetAdapter).Name")
    source = ps("(Get-NetIPAddress -InterfaceAlias '%s' -AddressFamily IPv4 | Select-Object -First 1).IPAddress" % uplink)
    clean = traces()
    expect("a clean machine: no trace of Spofy and the internet works directly", not any(clean.values()) and past(source), str(clean))

    real = build(BUNDLE, VERSION)
    old, new = build(BUNDLE, "0.0.0.1"), build(BUNDLE, "0.0.0.2")
    signed = sign(new)
    broken = build(variant("broken", spofy_core=STANDIN), "0.0.0.3")
    damaged = os.path.join(OUT, "Spofy-damaged-x64.exe")
    with open(new, "rb") as f, open(damaged, "wb") as d:
        d.write(f.read()[: os.path.getsize(new) * 2 // 3])

    site = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Site)
    threading.Thread(target=site.serve_forever, daemon=True).start()
    port = stand.free_port()
    remote = stand.server(port, uplink, {"protocol": "freedom", "settings": {"redirect": "127.0.0.1:%d" % site.server_address[1], "finalRules": [{"action": "allow", "ip": ["127.0.0.1/32"]}]}})
    stand.tls()
    pages, address = stand.page(open(stand.listed(port), "rb").read())

    code, took = setup(old)
    up = until(pipe_open, 30, 0.2)
    size = sum(os.path.getsize(os.path.join(d, f)) for d, _, fs in os.walk(HOME) for f in fs) if os.path.exists(HOME) else 0
    tray = until(stand.tray_window, 30, 0.1)
    expect("installed silently: the service runs, the tray is up and starts at logon, the window is in the Start menu and on the desktop",
           code == 0 and bool(up) and bool(tray) and stand.tray_icon(tray) and value(RUN, "Spofy") == '"%s"' % os.path.join(HOME, "spofy-tray.exe")
           and os.path.exists(SHORTCUT) and os.path.exists(DESKTOP) and value(ENTRY, "DisplayVersion") == "0.0.0.1",
           "code=%d s=%.1f on_disk=%.1f MB run=%s tray=%s" % (code, took, size / 1e6, value(RUN, "Spofy"), bool(tray)))
    if not up:
        return
    ui = Pipe(PIPE)
    added = ui.ask("subscribe", 60, url=address)
    at = time.time()
    on = ui.ask("connect", 60)
    took = time.time() - at
    hits = Site.hits
    expect("a subscription and a connection through the installed service: another program goes through the tunnel",
           bool(added) and added.get("ok") and bool(on) and on.get("ok") and away() and Site.hits > hits and not past(source),
           "connect=%.2f s %s %s" % (took, str(added)[:200], str(on)[:200]))
    expect("the Wintun driver is in the system while the tunnel is up", traces()["driver"])
    if "all" not in SKIP:
        launches(source, ui, old)
    elif os.path.exists(DESKTOP):
        os.remove(DESKTOP)
    print("     step: after the launches", flush=True)
    ui.ask("set", key="killSwitch", value=True)
    ui.ask("set", key="autoConnect", value=False)
    ui.close()

    window = subprocess.Popen([os.path.join(HOME, "spofy.exe")])
    shown = until(stand.window, 30, 0.2)
    before = owner(stand.tray_window())
    watch = Watch(source)
    began = time.time()
    code, took = setup(new)
    back = connected()
    outage, leaks = watch.stop(began)
    tray = until(lambda: owner(stand.tray_window()) not in (0, before), 30, 0.1)
    front = until(lambda: stand.window() and stand.user32.GetForegroundWindow() == stand.window(), 15, 0.1)
    expect("upgraded over itself with the kill switch and the window open: no reboot, the window closed, the tray replaced, the service connects by itself, nothing leaks, the desktop shortcut the person removed stays removed",
           code == 0 and bool(shown) and window.poll() is not None and bool(tray) and back and leaks == 0 and value(ENTRY, "DisplayVersion") == "0.0.0.2"
           and not os.path.exists(DESKTOP),
           "code=%d setup=%.1f s vpn_down=%.1f s leaks=%d window=%s tray=%s signed=%s reopened_in_front=%s" % (code, took, outage, leaks, window.poll(), bool(tray), signed, bool(front)))

    kept = files(HOME)
    code, took = setup(damaged)
    expect("a damaged installer is refused before anything is touched: the running version stays and keeps the tunnel",
           code == 1 and files(HOME) == kept and value(ENTRY, "DisplayVersion") == "0.0.0.2" and away(), "code=%d s=%.1f" % (code, took))

    kept = files(HOME)
    watch = Watch(source)
    began = time.time()
    code, took = setup(broken)
    back = connected()
    outage, leaks = watch.stop(began)
    left = files(HOME)
    expect("an upgrade whose service does not start: every file of the running version is put back, it connects by itself, nothing leaks",
           code == 9 and back and leaks == 0 and left == kept and value(ENTRY, "DisplayVersion") == "0.0.0.2" and not glob.glob(HOME + ".*"),
           "code=%d setup=%.1f s vpn_down=%.1f s leaks=%d changed=%s" % (code, took, outage, leaks, sorted(k for k in set(kept) | set(left) if kept.get(k) != left.get(k))[:5]))

    fresh = build(BUNDLE, "0.0.0.4")
    files_served = {os.path.basename(fresh): fresh}
    store, base = shelf(files_served)
    ui = Pipe(PIPE)
    current = (ui.ask("hello") or {}).get("snapshot", {}).get("build", 0)
    ui.close()
    signed_file, public = publish(fresh, "0.0.0.4", current + 1, base)
    files_served["update.json"] = signed_file
    restarted = service_env({"SPOFY_UPDATE_SOURCE": base + "update.json", "SPOFY_UPDATE_KEY": public})
    back = connected()
    ui = Pipe(PIPE)
    checked = ui.ask("checkUpdate", 60)
    offered = ui.wait(lambda m: m.get("ev") == "update" and (m.get("update") or {}).get("build") == current + 1, 30)
    fetched = ui.ask("fetchUpdate", 300)
    ready = ui.wait(lambda m: m.get("ev") == "update" and (m.get("update") or {}).get("file"), 30)
    watch = Watch(source)
    began = time.time()
    started = ui.ask("installUpdate", 30)
    ui.close()
    done = until(lambda: value(ENTRY, "DisplayVersion") == "0.0.0.4", 300, 0.5)
    up = connected(120)
    outage, leaks = watch.stop(began)
    took = time.time() - began
    expect("the service updates itself from a signed update file: it downloads and checks it, runs the installer, comes back on the new version and connects by itself, nothing leaks",
           current > 0 and restarted and back and all(r and r.get("ok") for r in (checked, fetched, started)) and bool(offered) and bool(ready) and bool(done) and up and leaks == 0,
           "build=%d check=%s fetch=%s install=%s offered=%s ready=%s version=%s update=%.1f s vpn_down=%.1f s leaks=%d"
           % (current, checked, fetched, started, bool(offered), bool(ready), value(ENTRY, "DisplayVersion"), took, outage, leaks))
    store.shutdown()

    window = subprocess.Popen([os.path.join(HOME, "spofy.exe")])
    until(stand.window, 30, 0.2)
    stand.keep_journal()
    code, took, gone = remove()
    free = until(lambda: past(source, 3), 20, 0.2)
    left = traces()
    expect("uninstalled with the kill switch on and the window open: the internet works directly and nothing is left",
           code == 0 and gone and bool(free) and left == clean and not stand.tray_window() and not glob.glob(HOME + "*"),
           "code=%d s=%.1f left=%s tray=%s" % (code, took, {k: v for k, v in left.items() if v}, bool(stand.tray_window())))
    print("     installer %.2f MB (%s), on disk %.1f MB" % (os.path.getsize(real) / 1e6, os.path.basename(real), size / 1e6))
    pages.shutdown()
    site.shutdown()
    remote.p.kill()
    subprocess.run(["certutil", "-delstore", "Root", "Spofy Stand CA"], capture_output=True)


def rescue(after):
    time.sleep(after)
    stuck = os.path.join(OUT, "stuck.txt")
    with open(stuck, "w") as f:
        faulthandler.dump_traceback(file=f, all_threads=True)
    print("STUCK after %d s, the threads:\n%s" % (after, open(stuck).read()), flush=True)
    print("     traces before the rescue: %s" % traces(), flush=True)
    if os.path.exists(os.path.join(HOME, "uninstall.exe")):
        try:
            subprocess.run([os.path.join(HOME, "uninstall.exe"), "-uninstall", "-quiet", "-log", os.path.join(OUT, "rescue-uninstall.log")], timeout=180)
        except subprocess.TimeoutExpired:
            print("     the rescue uninstall did not finish in 180 s", flush=True)
    subprocess.run(["reg", "add", r"HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings", "/v", "ProxyEnable", "/t", "REG_DWORD", "/d", "0", "/f"], capture_output=True)
    print("     traces after the rescue: %s" % traces(), flush=True)
    for name in sorted(os.listdir(OUT)):
        if name.endswith(".log"):
            print("--- %s\n%s" % (name, open(os.path.join(OUT, name), encoding="utf-8", errors="replace").read()[-2500:]), flush=True)
    os._exit(3)


began = time.time()
threading.Thread(target=rescue, args=(360,), daemon=True).start()
try:
    scenario()
finally:
    if failed:
        print("--- service\n" + stand.journal[0])
        for name in sorted(os.listdir(OUT)):
            if name.endswith(".log"):
                print("--- %s\n%s" % (name, open(os.path.join(OUT, name), encoding="utf-8", errors="replace").read()[-2500:]))
print("%s in %.0f s, %d failed" % ("INSTALLER OK" if not failed else "INSTALLER FAILED", time.time() - began, len(failed)))
sys.exit(1 if failed else 0)
