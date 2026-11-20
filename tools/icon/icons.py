import math
import re
import struct
import sys
import zlib
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
LOGO = ROOT / "tools/icon/logo.svg"
RES = ROOT / "app/android/app/src/main/res"
ICO = ROOT / "app/windows/runner/resources/app_icon.ico"
TRAY = ROOT / "core/cmd/spofy-tray/icons"
BRAND = (0x25, 0x63, 0xEB)
WHITE = (0xFF, 0xFF, 0xFF)
GREY = (0x64, 0x74, 0x8B)
AMBER = (0xFB, 0xBF, 0x24)
LEGACY = {"mdpi": 48, "hdpi": 72, "xhdpi": 96, "xxhdpi": 144, "xxxhdpi": 192}
WINDOWS = (16, 20, 24, 32, 40, 48, 64, 96, 128, 256)
TRAY_SIZES = (16, 20, 24, 32, 40, 48, 64)
ROWS = 16


def logo():
    data = re.search(r'\sd="([^"]+)"', LOGO.read_text()).group(1)
    tokens = re.findall(r"[A-Za-z]|-?(?:\d+\.?\d*|\.\d+)(?:e-?\d+)?", data)
    shapes, shape, at, start, i, op = [], [], (0.0, 0.0), (0.0, 0.0), 0, None
    take = lambda n: [float(t) for t in tokens[i:i + n]]
    while i < len(tokens):
        if tokens[i].isalpha():
            op = tokens[i]
            i += 1
            if op == "Z":
                if shape:
                    shapes.append(shape)
                shape, at = [], start
                continue
        if op == "M":
            if shape:
                shapes.append(shape)
            at = start = tuple(take(2))
            shape = [at]
            i += 2
            op = "L"
        elif op == "L":
            at = tuple(take(2))
            shape.append(at)
            i += 2
        elif op == "H":
            at = (take(1)[0], at[1])
            shape.append(at)
            i += 1
        elif op == "V":
            at = (at[0], take(1)[0])
            shape.append(at)
            i += 1
        elif op == "C":
            x1, y1, x2, y2, x3, y3 = take(6)
            x0, y0 = at
            for k in range(1, 49):
                t = k / 48
                u = 1 - t
                shape.append((
                    u * u * u * x0 + 3 * u * u * t * x1 + 3 * u * t * t * x2 + t * t * t * x3,
                    u * u * u * y0 + 3 * u * u * t * y1 + 3 * u * t * t * y2 + t * t * t * y3,
                ))
            at = (x3, y3)
            i += 6
        else:
            raise SystemExit("logo.svg: unsupported path command %r" % op)
    if shape:
        shapes.append(shape)
    return shapes


def bounds(shapes):
    xs = [x for s in shapes for x, _ in s]
    ys = [y for s in shapes for _, y in s]
    return min(xs), min(ys), max(xs), max(ys)


def rounded(x0, y0, x1, y1, r):
    points = []
    for cx, cy, a in ((x1 - r, y0 + r, -90), (x1 - r, y1 - r, 0), (x0 + r, y1 - r, 90), (x0 + r, y0 + r, 180)):
        for k in range(25):
            t = math.radians(a + 90 * k / 24)
            points.append((cx + r * math.cos(t), cy + r * math.sin(t)))
    return [points]


def coverage(shapes, size):
    edges = []
    for s in shapes:
        for (xa, ya), (xb, yb) in zip(s, s[1:] + s[:1]):
            if ya != yb:
                edges.append((min(ya, yb), max(ya, yb), xa, ya, (xb - xa) / (yb - ya), 1 if yb > ya else -1))
    cells = [[0.0] * size for _ in range(size)]
    for y in range(size):
        row = cells[y]
        near = [e for e in edges if e[0] < y + 1 and e[1] > y]
        for sub in range(ROWS):
            sy = y + (sub + 0.5) / ROWS
            hits = sorted((xa + (sy - ya) * slope, way) for top, bottom, xa, ya, slope, way in near if top <= sy < bottom)
            winding = 0
            for (xa, way), (xb, _) in zip(hits, hits[1:]):
                winding += way
                if winding and xb > xa:
                    fill(row, max(xa, 0.0), min(xb, float(size)), size)
    return cells


def fill(row, xa, xb, size):
    if xb <= xa:
        return
    ia, ib = int(xa), int(xb)
    if ia == ib:
        row[ia] += (xb - xa) / ROWS
        return
    row[ia] += (ia + 1 - xa) / ROWS
    for i in range(ia + 1, min(ib, size)):
        row[i] += 1 / ROWS
    if ib < size:
        row[ib] += (xb - ib) / ROWS


def place(shapes, size, height):
    x0, y0, x1, y1 = bounds(shapes)
    scale = height / (y1 - y0)
    dx = size / 2 - (x0 + x1) / 2 * scale
    dy = size / 2 - (y0 + y1) / 2 * scale
    return [[(x * scale + dx, y * scale + dy) for x, y in s] for s in shapes]


def tile(size, margin, radius, height, back_color=BRAND):
    inset = size * margin
    back = coverage(rounded(inset, inset, size - inset, size - inset, size * radius), size)
    mark = coverage(place(logo(), size, size * height), size)
    pixels = bytearray()
    for y in range(size):
        for x in range(size):
            a = min(back[y][x], 1.0)
            m = min(mark[y][x], 1.0)
            pixels += bytes(round(b * (1 - m) + w * m) for b, w in zip(back_color, WHITE))
            pixels.append(round(a * 255))
    return size, pixels


def png(size, pixels):
    chunk = lambda kind, body: struct.pack(">I", len(body)) + kind + body + struct.pack(">I", zlib.crc32(kind + body))
    raw = b"".join(b"\0" + bytes(pixels[y * size * 4:(y + 1) * size * 4]) for y in range(size))
    return (
        b"\x89PNG\r\n\x1a\n"
        + chunk(b"IHDR", struct.pack(">IIBBBBB", size, size, 8, 6, 0, 0, 0))
        + chunk(b"IDAT", zlib.compress(raw, 9))
        + chunk(b"IEND", b"")
    )


def unpng(data):
    pos, idat = 8, b""
    while pos < len(data):
        n, kind = struct.unpack(">I4s", data[pos:pos + 8])
        body = data[pos + 8:pos + 8 + n]
        pos += 12 + n
        if kind == b"IHDR":
            size, height, depth, color = struct.unpack(">IIBB", body[:10])
            if (size, depth, color) != (height, 8, 6):
                return None
        elif kind == b"IDAT":
            idat += body
    raw = zlib.decompress(idat)
    out, prev, i, stride = bytearray(), bytearray(size * 4), 0, size * 4
    for _ in range(size):
        kind, line = raw[i], bytearray(raw[i + 1:i + 1 + stride])
        i += 1 + stride
        for x in range(stride):
            a = line[x - 4] if x >= 4 else 0
            b = prev[x]
            c = prev[x - 4] if x >= 4 else 0
            if kind == 1:
                line[x] = (line[x] + a) & 255
            elif kind == 2:
                line[x] = (line[x] + b) & 255
            elif kind == 3:
                line[x] = (line[x] + (a + b) // 2) & 255
            elif kind == 4:
                pa, pb, pc = abs(b - c), abs(a - c), abs(a + b - 2 * c)
                line[x] = (line[x] + (a if pa <= pb and pa <= pc else b if pb <= pc else c)) & 255
        out += line
        prev = line
    return size, out


def ico(images):
    head = struct.pack("<HHH", 0, 1, len(images))
    offset = len(head) + 16 * len(images)
    table, body = b"", b""
    for size, data in images:
        table += struct.pack("<BBBBHHII", size % 256, size % 256, 0, 0, 1, 32, len(data), offset + len(body))
        body += data
    return head + table + body


def unico(data):
    count = struct.unpack("<H", data[4:6])[0]
    found = []
    for k in range(count):
        _, _, _, _, _, _, length, offset = struct.unpack("<BBBBHHII", data[6 + 16 * k:22 + 16 * k])
        found.append(unpng(data[offset:offset + length]))
    return found


def vector():
    shapes = logo()
    x0, y0, x1, y1 = bounds(shapes)
    scale = 44 / (y1 - y0)
    data = re.search(r'\sd="([^"]+)"', LOGO.read_text()).group(1)
    return (
        '<?xml version="1.0" encoding="utf-8"?>\n'
        '<vector xmlns:android="http://schemas.android.com/apk/res/android"\n'
        '    android:width="108dp"\n'
        '    android:height="108dp"\n'
        '    android:viewportWidth="108"\n'
        '    android:viewportHeight="108">\n'
        '    <group\n'
        '        android:scaleX="%.6f"\n'
        '        android:scaleY="%.6f"\n'
        '        android:translateX="%.4f"\n'
        '        android:translateY="%.4f">\n'
        '        <path\n'
        '            android:fillColor="#FFFFFFFF"\n'
        '            android:pathData="%s"/>\n'
        '    </group>\n'
        '</vector>\n'
    ) % (scale, scale, 54 - (x0 + x1) / 2 * scale, 54 - (y0 + y1) / 2 * scale, data)


def legacy(size):
    return tile(size, 1 / 12, 0.2, 0.56)


def windows(size):
    return tile(size, 1 / 32, 0.22, 0.66 if size < 32 else 0.6)


def tray(size, back_color, fade=1.0, dot=None):
    _, pixels = tile(size, 1 / 32, 0.22, 0.66 if size < 32 else 0.6, back_color)
    r = size * 0.22
    gap = max(1.0, size * 0.08)
    c = size - r
    ring = coverage(rounded(c - r - gap, c - r - gap, c + r + gap, c + r + gap, r + gap), size) if dot else None
    spot = coverage(rounded(c - r, c - r, c + r, c + r, r), size) if dot else None
    out = bytearray()
    for y in range(size):
        for x in range(size):
            i = (y * size + x) * 4
            rgb, a = pixels[i:i + 3], pixels[i + 3] / 255 * fade
            if dot:
                s = min(spot[y][x], 1.0)
                a *= 1 - min(ring[y][x], 1.0)
                total = s + a * (1 - s)
                rgb = [(d * s + v * a * (1 - s)) / total if total else 0 for v, d in zip(rgb, dot)]
                a = total
            out += bytes(round(v) for v in rgb)
            out.append(round(a * 255))
    return size, out


def wanted():
    files = {RES / "drawable/ic_launcher_foreground.xml": vector().encode()}
    for name, size in LEGACY.items():
        files[RES / ("mipmap-%s/ic_launcher.png" % name)] = legacy(size)
    files[ICO] = [windows(size) for size in WINDOWS]
    files[TRAY / "on.ico"] = [tray(size, BRAND) for size in TRAY_SIZES]
    files[TRAY / "off.ico"] = [tray(size, GREY) for size in TRAY_SIZES]
    files[TRAY / "busy.ico"] = [tray(size, BRAND, fade=0.5) for size in TRAY_SIZES]
    files[TRAY / "alert.ico"] = [tray(size, BRAND, dot=AMBER) for size in TRAY_SIZES]
    return files


def main():
    mode = sys.argv[1] if len(sys.argv) > 1 else ""
    if mode not in ("write", "check"):
        raise SystemExit("usage: icons.py write|check")
    bad = []
    for path, want in wanted().items():
        if mode == "write":
            if isinstance(want, bytes):
                path.write_bytes(want)
            elif isinstance(want, list):
                path.write_bytes(ico([(size, png(size, pixels)) for size, pixels in want]))
            else:
                path.write_bytes(png(*want))
            continue
        have = path.read_bytes() if path.exists() else None
        if isinstance(want, bytes):
            same = have == want
        elif isinstance(want, list):
            same = have is not None and unico(have) == want
        else:
            same = have is not None and unpng(have) == want
        if not same:
            bad.append(str(path.relative_to(ROOT)))
    if bad:
        print("ICONS DIFFER: " + ", ".join(bad))
        raise SystemExit(1)
    print("ICONS OK")


if __name__ == "__main__":
    main()
