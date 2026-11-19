import fcntl, json, os, socket, struct, subprocess, sys

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
    for family in state["rules"]:
        ip(family, "rule", "del", "priority", "100", check=False)
    state["rules"] = []


def tunnel(spec):
    fd = os.open("/dev/net/tun", os.O_RDWR)
    try:
        name = fcntl.ioctl(fd, 0x400454CA, struct.pack("16sH", (spec.get("name") or "vpn%d").encode(), 0x0001 | 0x1000))[:16].rstrip(b"\0").decode()
        ip("link", "set", name, "mtu", str(spec.get("mtu") or 1500), "up")
        unrule()
        for family, zero in (("-4", "."), ("-6", ":")):
            addresses = [a for a in spec.get("addresses", []) if zero in a]
            routes = [r for r in spec.get("routes", []) if zero in r]
            try:
                for a in addresses:
                    ip(family, "addr", "add", a, "dev", name)
                for r in routes:
                    ip(family, "route", "add", r, "dev", name, "table", TABLE)
                if routes:
                    ip(family, "rule", "add", "not", "uidrange", "%s-%s" % (APP, APP), "lookup", TABLE, "priority", "100")
                    state["rules"].append(family)
            except RuntimeError:
                if family == "-4":
                    raise
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
                note(op="untun", device=state["device"])
                state["device"] = None
            s.sendall((json.dumps(answer) + "\n").encode())
    unrule()


main()
