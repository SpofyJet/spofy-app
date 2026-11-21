import argparse
import base64
import hashlib
import json
import os
import re
import shutil
import subprocess
import sys
import time
import urllib.error
import urllib.request
import zipfile

REPO = "SpofyJet/spofy-app"
API = "https://api.github.com/repos/" + REPO
ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
ABIS = ("arm64-v8a", "armeabi-v7a", "x86_64")


def fail(text):
    raise SystemExit("RELEASE FAILED: " + text)


class Stay(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *args, **kw):
        return None


def call(method, url, body=None, kind="application/json", limit=60):
    data = body if isinstance(body, bytes) or body is None else json.dumps(body).encode()
    req = urllib.request.Request(url, data=data, method=method, headers={
        "Authorization": "token " + os.environ["GH_TOKEN"], "Accept": "application/vnd.github+json", "User-Agent": "spofy-release", "Content-Type": kind})
    try:
        with urllib.request.build_opener(Stay).open(req, timeout=limit) as r:
            text = r.read()
            return r.status, json.loads(text) if text else {}
    except urllib.error.HTTPError as e:
        if e.code in (301, 302, 307):
            return e.code, {"location": e.headers.get("Location")}
        return e.code, {"error": e.read().decode(errors="replace")[:500]}


def fetch(url, limit=300):
    for attempt in range(4):
        try:
            with urllib.request.urlopen(urllib.request.Request(url, headers={"User-Agent": "spofy-release"}), timeout=limit) as r:
                return r.read()
        except (urllib.error.URLError, TimeoutError, ConnectionError) as e:
            if attempt == 3:
                fail("download %s: %s" % (url, e))
            time.sleep(5)


def run(*cmd, **kw):
    r = subprocess.run(cmd, capture_output=True, text=True, **kw)
    if r.returncode != 0:
        fail("%s: %s %s" % (os.path.basename(cmd[0]), r.stdout[-1500:], r.stderr[-1500:]))
    return r.stdout


def digest(path):
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


def green(sha):
    status, body = call("GET", API + "/actions/runs?head_sha=%s&per_page=30" % sha)
    if status != 200:
        fail("runs: %s" % status)
    runs = {}
    for r in sorted(body.get("workflow_runs", []), key=lambda r: r["id"]):
        runs[r["name"]] = r
    for name in ("core", "windows"):
        r = runs.get(name)
        if not r or r["status"] != "completed" or r["conclusion"] != "success":
            fail("workflow %s for %s is %s" % (name, sha[:7], (r or {}).get("conclusion") or (r or {}).get("status") or "missing"))
    return runs


def artifact(run, name, into):
    status, body = call("GET", API + "/actions/runs/%d/artifacts?per_page=50" % run["id"])
    found = [a for a in body.get("artifacts", []) if a["name"] == name and not a["expired"]]
    if status != 200 or len(found) != 1:
        fail("artifact %s of run %d: %s" % (name, run["id"], status))
    status, moved = call("GET", found[0]["archive_download_url"])
    if status != 302:
        fail("artifact %s: %s" % (name, status))
    path = os.path.join(into, name + ".zip")
    with open(path, "wb") as f:
        f.write(fetch(moved["location"]))
    folder = os.path.join(into, name)
    with zipfile.ZipFile(path) as z:
        for entry in z.namelist():
            if "/" in entry or entry.startswith(".") or not re.fullmatch(r"[A-Za-z0-9._-]+", entry):
                continue
            z.extract(entry, folder)
    return folder


def upload(release, path, name):
    url = "https://uploads.github.com/repos/%s/releases/%d/assets?name=%s" % (REPO, release, name)
    data = open(path, "rb").read()
    for attempt in range(4):
        status, body = call("POST", url, data, "application/octet-stream", limit=600)
        if status == 201 and body.get("size") == len(data):
            return
        _, listed = call("GET", API + "/releases/%d/assets?per_page=50" % release)
        for a in listed if isinstance(listed, list) else []:
            if a["name"] == name:
                call("DELETE", API + "/releases/assets/%d" % a["id"])
        time.sleep(5)
    fail("upload %s: %s" % (name, status))


def main():
    p = argparse.ArgumentParser()
    p.add_argument("sha")
    p.add_argument("notes")
    p.add_argument("--rollout", type=int, default=100)
    p.add_argument("--min", type=int, default=0)
    p.add_argument("--publish", action="store_true")
    o = p.parse_args()
    sign_key = os.environ.get("SPOFY_SIGN_KEY", "")
    update_key = os.environ.get("SPOFY_UPDATE_KEY", "")
    if not os.path.isdir(sign_key) or not os.path.isfile(update_key) or not os.path.isfile(update_key[:-4] + ".pub"):
        fail("SPOFY_SIGN_KEY must be the folder of the app key and SPOFY_UPDATE_KEY the update key file with its .pub beside it")
    sha = run("git", "-C", ROOT, "rev-parse", o.sha + "^{commit}").strip()
    run("git", "-C", ROOT, "fetch", "-q", "https://github.com/%s.git" % REPO, "main")
    if subprocess.run(["git", "-C", ROOT, "merge-base", "--is-ancestor", sha, "FETCH_HEAD"]).returncode != 0:
        fail("%s is not on main" % sha[:7])
    pub = run("git", "-C", ROOT, "show", sha + ":app/pubspec.yaml")
    m = re.search(r"^version: (\d+\.\d+\.\d+)\+(\d+)$", pub, re.M)
    if not m:
        fail("no version in app/pubspec.yaml")
    version, build = m.group(1), int(m.group(2))
    public = open(update_key[:-4] + ".pub").read().strip()
    if '"%s"' % public not in run("git", "-C", ROOT, "show", sha + ":core/cmd/spofy-core/release.go"):
        fail("the update key is not one the app trusts")
    tag = "v" + version
    status, _ = call("GET", API + "/releases/tags/" + tag)
    if status != 404:
        fail("release %s already exists" % tag)
    status, latest = call("GET", API + "/releases/latest")
    if status == 200:
        before = json.loads(base64.b64decode(json.loads(fetch("https://github.com/%s/releases/latest/download/update.json" % REPO))["manifest"]))["build"]
        if build <= before:
            fail("build %d is not above the published build %d of %s" % (build, before, latest["tag_name"]))
    work = os.path.join(ROOT, "dist", "release-" + version)
    if os.path.exists(work):
        fail(work + " exists")
    ci, out = os.path.join(work, "ci"), os.path.join(work, "out")
    os.makedirs(ci)
    os.makedirs(out)
    runs = green(sha)
    apks = artifact(runs["core"], "apk", ci)
    installer = artifact(runs["windows"], "installer", ci)
    signed = os.path.join(work, "signed")
    if "SIGN OK" not in run(os.path.join(ROOT, "tools", "android", "sign.sh"), apks, signed, env=dict(os.environ, SPOFY_SIGN_KEY=sign_key)):
        fail("sign.sh")
    files = {}
    for abi in ABIS:
        name = "Spofy-%s-%s.apk" % (version, abi)
        shutil.copy(os.path.join(signed, "app-%s-release.apk" % abi), os.path.join(out, name))
        files["android-" + abi] = name
    exes = [f for f in os.listdir(installer) if re.fullmatch(r"Spofy-%s\.\d+-x64\.exe" % re.escape(version), f)]
    if len(exes) != 1:
        fail("installer of %s: %s" % (version, sorted(os.listdir(installer))))
    name = "Spofy-%s-x64.exe" % version
    shutil.copy(os.path.join(installer, exes[0]), os.path.join(out, name))
    files["windows-x64"] = name
    tool = os.path.join(work, "spofy-release")
    run(os.path.join(ROOT, "tools", "go.sh"), "go", "build", "-trimpath", "-o", "/src/spofy-app/" + os.path.relpath(tool, ROOT), "./cmd/spofy-release")
    base = "https://github.com/%s/releases/download/%s/" % (REPO, tag)
    args = [tool, "manifest", "-version", version, "-build", str(build), "-notes-file", o.notes, "-min", str(o.min), "-rollout", str(o.rollout), "-url-base", base, "-out", os.path.join(work, "manifest.json")]
    for platform, file in files.items():
        args += ["-file", "%s=%s" % (platform, os.path.join(out, file))]
    run(*args)
    run(tool, "sign", "-key", update_key, "-manifest", os.path.join(work, "manifest.json"), "-out", os.path.join(out, "update.json"))
    print(run(tool, "verify", "-pub", public, "-in", os.path.join(out, "update.json")).strip())
    for f in sorted(os.listdir(out)):
        print("  %-32s %6.1f MB  %s" % (f, os.path.getsize(os.path.join(out, f)) / 1e6, digest(os.path.join(out, f))[:16]))
    if not o.publish:
        print("READY %s (%s, build %d) in %s; nothing published" % (tag, sha[:7], build, out))
        return
    status, rel = call("POST", API + "/releases", {"tag_name": tag, "target_commitish": sha, "name": "Spofy " + version, "body": open(o.notes, encoding="utf-8").read(), "draft": True})
    if status != 201:
        fail("create release: %s %s" % (status, rel))
    for f in [*files.values(), "update.json"]:
        upload(rel["id"], os.path.join(out, f), f)
    status, rel = call("PATCH", API + "/releases/%d" % rel["id"], {"draft": False, "make_latest": "true"})
    if status != 200:
        fail("publish release: %s" % status)
    signed = open(os.path.join(out, "update.json"), "rb").read()
    for _ in range(24):
        if fetch("https://github.com/%s/releases/latest/download/update.json" % REPO) == signed:
            break
        time.sleep(5)
    else:
        fail("the latest update.json on GitHub is not the one signed here after 2 minutes")
    manifest = json.load(open(os.path.join(work, "manifest.json")))
    for platform, f in manifest["files"].items():
        for url in f["urls"]:
            if hashlib.sha256(fetch(url)).hexdigest() != f["sha256"]:
                fail("%s from %s differs" % (platform, url))
    print("PUBLISHED %s %s: update.json and %d files match what was signed" % (tag, rel["html_url"], len(files)))


main()
