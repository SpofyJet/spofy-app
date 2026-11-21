import os
import shutil
import subprocess
import sys


def main():
    package, description, version, icon, manifest = sys.argv[1:6]
    numbers = [int(p) for p in (version.split(".") + ["0"] * 4)[:4]]
    name = os.path.basename(os.path.normpath(package)) + ".exe"
    tool = shutil.which("windres")
    if not tool:
        raise SystemExit("windres is not on this machine")
    slash = lambda p: os.path.abspath(p).replace("\\", "/")
    rc = os.path.join(package, "resources.rc")
    with open(rc, "w", encoding="utf-8") as f:
        f.write('1 24 "%s"\n' % slash(manifest))
        f.write('1 ICON "%s"\n' % slash(icon))
        f.write("1 VERSIONINFO\nFILEVERSION %s\nPRODUCTVERSION %s\nFILEOS 0x40004\nFILETYPE 0x1\nBEGIN\n" % ((",".join(map(str, numbers)),) * 2))
        f.write('  BLOCK "StringFileInfo"\n  BEGIN\n    BLOCK "040904b0"\n    BEGIN\n')
        for key, value in [("CompanyName", "Spofy"), ("FileDescription", description), ("FileVersion", version),
                           ("ProductName", "Spofy"), ("ProductVersion", version), ("OriginalFilename", name)]:
            f.write('      VALUE "%s", "%s"\n' % (key, value))
        f.write('    END\n  END\n  BLOCK "VarFileInfo"\n  BEGIN\n    VALUE "Translation", 0x409, 1200\n  END\nEND\n')
    out = os.path.join(package, "rsrc_windows_amd64.syso")
    subprocess.run([tool, "-i", rc, "-o", out, "-O", "coff", "-F", "pe-x86-64", "-c", "65001"], check=True)
    print("%s: %d bytes of resources for %s %s" % (out, os.path.getsize(out), name, version))


main()
