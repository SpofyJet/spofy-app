import hashlib
import io
import re
import sys
import urllib.request
import xml.etree.ElementTree as ET
import zipfile

GOOGLE = "https://dl.google.com/android/repository/"
MAVEN = "https://dl.google.com/android/maven2/"
FLUTTER = "https://storage.googleapis.com/download.flutter.io/"


def get(url):
    return urllib.request.urlopen(urllib.request.Request(url, headers={"User-Agent": "curl"}), timeout=120).read()


def platform(level, out):
    root = ET.fromstring(get(GOOGLE + "repository2-3.xml"))
    best = None
    for package in root.iter("remotePackage"):
        if package.get("path") != "platforms;android-%s" % level:
            continue
        revision = int(package.find("revision/major").text)
        archive = package.find("archives/archive/complete")
        if best is None or revision > best[0]:
            best = (revision, archive.find("url").text, archive.find("checksum").text)
    if best is None:
        raise SystemExit("no platform %s" % level)
    body = get(GOOGLE + best[1])
    if hashlib.sha1(body).hexdigest() != best[2]:
        raise SystemExit("platform archive does not match its checksum")
    with zipfile.ZipFile(io.BytesIO(body)) as z:
        name = [n for n in z.namelist() if n.endswith("/android.jar")][0]
        open(out, "wb").write(z.read(name))


def embedding(revision, folder):
    base = "io/flutter/flutter_embedding_debug/1.0.0-%s/flutter_embedding_debug-1.0.0-%s" % (revision, revision)
    open(folder + "/embedding.jar", "wb").write(get(FLUTTER + base + ".jar"))
    pom = get(FLUTTER + base + ".pom").decode()
    for group, name, version in re.findall(r"<groupId>(androidx[^<]*)</groupId>\s*<artifactId>([^<]*)</artifactId>\s*<version>([^<]*)</version>", pom):
        path = "%s/%s/%s/%s-%s" % (group.replace(".", "/"), name, version, name, version)
        for kind in (".jar", ".aar"):
            try:
                body = get(MAVEN + path + kind)
            except Exception:
                continue
            if kind == ".aar":
                with zipfile.ZipFile(io.BytesIO(body)) as z:
                    body = z.read("classes.jar")
            open("%s/%s.jar" % (folder, name), "wb").write(body)
            break


if __name__ == "__main__":
    {"platform": platform, "embedding": embedding}[sys.argv[1]](sys.argv[2], sys.argv[3])
