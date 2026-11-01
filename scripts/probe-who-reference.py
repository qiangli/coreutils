#!/usr/bin/env python3
"""Bounded native-ABI probe; observes installed references without source access."""
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess

root = Path(__file__).resolve().parent.parent
out = Path(os.environ["EVIDENCE_DIR"])
out.mkdir(parents=True, exist_ok=True)
source = (root / "cmds/who/testdata/mkutmp_linux.c").read_text()
providers = list(dict.fromkeys(path for name in ("/usr/bin/who", "gnuwho")
                               if (path := shutil.which(name))))
if not providers:
    raise SystemExit("No installed who reference")

def capture(argv):
    result = subprocess.run(argv, text=True, capture_output=True, timeout=20,
                            env=dict(os.environ, TZ="UTC", LC_ALL="C"))
    return {"argv": argv, "exit": result.returncode,
            "stdout": result.stdout, "stderr": result.stderr}

report = {"live_parent_pid": os.getpid(), "providers": [], "observations": []}
profile = Path("/etc/apparmor.d/who")
if profile.is_file():
    (out / "apparmor-who-profile.txt").write_text(profile.read_text())
# Same bytes under a temporary executable path isolate path-attached policy.
# This does not change or disable any installed AppArmor profile.
for provider in list(providers):
    target = out / "reference-copies" / Path(provider).name
    target.parent.mkdir(exist_ok=True)
    shutil.copyfile(provider, target)
    target.chmod(0o755)
    providers.append(str(target))
for provider in providers:
    report["providers"].append({"path": provider, "realpath": os.path.realpath(provider),
        "sha256": hashlib.sha256(Path(provider).read_bytes()).hexdigest(),
        "version": capture([provider, "--version"]),
        "package": capture(["dpkg-query", "-S", os.path.realpath(provider)])})
for kind in ("fixed", "live"):
    fixture_source = source
    if kind == "live":
        fixture_source = "#include <unistd.h>\n" + source.replace(
            'put(f, USER_PROCESS, 333,', 'put(f, USER_PROCESS, getppid(),')
    cfile, generator, fixture = (out / (kind + suffix) for suffix in (".c", "-mkutmp", ".utmp"))
    cfile.write_text(fixture_source)
    for argv in (["cc", "-Wall", "-Wextra", "-o", str(generator), str(cfile)],
                 [str(generator), str(fixture)]):
        result = capture(argv)
        if result["exit"]:
            raise SystemExit(json.dumps(result))
    if kind == "fixed" and shutil.which("strace"):
        report["trace"] = capture(["strace", "-f", "-e", "trace=openat", "-o",
            str(out / "reference-openat.txt"), "/usr/bin/who", "-b", str(fixture)])
    for provider in providers:
        for args in (["-s", "-u"], ["-T"], ["-b"], ["-r"], ["-t"], ["-T", "-l"]):
            report["observations"].append({"fixture": kind,
                **capture([provider, *args, str(fixture)])})
(out / "who-reference.json").write_text(json.dumps(report, indent=2) + "\n")
print(json.dumps(report, indent=2))
