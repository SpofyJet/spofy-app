import errno, fcntl, json, os, socket, struct, subprocess, sys, time

PATH, APP, WORK = sys.argv[1], sys.argv[2], sys.argv[3]
TABLE = "100"
state = {"device": None, "rules": []}


def note(**fields):
    with open(WORK + "/platform.log", "a") as out:
        out.write(json.dumps(fields) + "\n")


def ip(*args, check=True):
    r = subprocess.run(["ip"] + list(args), capture_output=True, text=True)
    if check and r.returncode != 0:
        raise RuntimeError("ip %s: %s" % (" ".join(args), r.stderr.strip()))
    return r.returncode == 0


def unrule():
    for rule in state["rules"]:
        ip(rule[0], "rule", "del", *rule[1:], check=False)
    state["rules"] = []


def fallback(verb):
    for family in ("-4", "-6"):
        ip(family, "route", verb, "unreachable", "default", "metric", "4000", "table", TABLE, check=False)


def create(fd, wanted):
    for attempt in range(40):
        try:
            return fcntl.ioctl(fd, 0x400454CA, struct.pack("16sH", (wanted or "vpn%d").encode(), 0x0001 | 0x1000))[:16].rstrip(b"\0").decode()
        except OSError as e:
            if e.errno != errno.EBUSY or attempt == 39:
                raise
            time.sleep(0.05)


def tunnel(spec):
    if state["device"]:
        fallback("replace")
        ip("link", "del", state["device"], check=False)
    fd = os.open("/dev/net/tun", os.O_RDWR)
    try:
        name = create(fd, spec.get("name"))
        ip("link", "set", name, "mtu", str(spec.get("mtu") or 1500), "up")
        apps = spec.get("apps") or {}
        users = ["%s-%s" % (u, u) for u in apps.get("list", []) if u.isdigit() and u != APP]
        wanted = []
        for family, zero in (("-4", "."), ("-6", ":")):
            addresses = [a for a in spec.get("addresses", []) if zero in a]
            routes = [r for r in spec.get("routes", []) if zero in r]
            mine = []
            if routes and apps.get("mode") == "only" and users:
                mine = [(family, "uidrange", u, "lookup", TABLE, "priority", "100") for u in users]
            elif routes:
                if apps.get("mode") == "except":
                    mine = [(family, "uidrange", u, "lookup", "main", "priority", "90") for u in users]
                mine.append((family, "not", "uidrange", "%s-%s" % (APP, APP), "lookup", TABLE, "priority", "100"))
            try:
                for a in addresses:
                    ip(family, "addr", "add", a, "dev", name)
                for r in routes:
                    ip(family, "route", "replace", r, "dev", name, "table", TABLE)
                for rule in mine:
                    if rule not in state["rules"]:
                        ip(rule[0], "rule", "add", *rule[1:])
                    wanted.append(rule)
            except RuntimeError:
                if family == "-4":
                    raise
        for rule in state["rules"]:
            if rule not in wanted:
                ip(rule[0], "rule", "del", *rule[1:], check=False)
        state["rules"] = wanted
        state["device"] = name
        return fd, name
    except Exception:
        os.close(fd)
        raise


def main():
    s = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    s.connect(PATH)
    s.sendall(b'{"id":1,"op":"platform"}\n')
    buffered = b""
    while True:
        chunk = s.recv(65536)
        if not chunk:
            break
        buffered += chunk
        while b"\n" in buffered:
            line, buffered = buffered.split(b"\n", 1)
            m = json.loads(line)
            if "ask" not in m:
                continue
            answer = {"answer": m["ask"], "ok": True}
            if os.path.exists(WORK + "/platform.refuse"):
                note(op=m["op"], refused=True)
                s.sendall((json.dumps({"answer": m["ask"], "ok": False, "error": "the user did not allow the tunnel"}) + "\n").encode())
                continue
            if m["op"] == "tun":
                try:
                    fd, name = tunnel(m.get("tun") or {})
                except Exception as e:
                    note(op="tun", error=str(e))
                    s.sendall((json.dumps({"answer": m["ask"], "ok": False, "error": str(e)}) + "\n").encode())
                    continue
                answer["file"] = True
                note(op="tun", device=name, spec=m.get("tun"))
                socket.send_fds(s, [(json.dumps(answer) + "\n").encode()], [fd])
                os.close(fd)
                continue
            if m["op"] == "untun":
                unrule()
                fallback("del")
                note(op="untun", device=state["device"])
                state["device"] = None
            s.sendall((json.dumps(answer) + "\n").encode())
    unrule()


main()
