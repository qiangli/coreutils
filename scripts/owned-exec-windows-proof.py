"""Focused Windows product proof for nested coreutils owned-child launches."""

import os
import pathlib
import shutil
import subprocess
import sys
import tempfile

binary = pathlib.Path(sys.argv[1]).resolve()
payload = "é" * 100000  # 200,000 UTF-8 bytes, above CreateProcess argv limit.

with tempfile.TemporaryDirectory(prefix="coreutils-owned-exec-") as directory:
    applets = {}
    for name in ("xargs", "printf", "env", "printenv"):
        target = pathlib.Path(directory) / (name + ".exe")
        try:
            os.link(binary, target)
        except OSError:
            shutil.copyfile(binary, target)
        applets[name] = str(target)

    cases = [
        (
            "xargs -> printf argv",
            [applets["xargs"], "-0", applets["printf"], "%s"],
            payload.encode() + b"\0",
            payload.encode(),
        ),
        (
            "xargs -> env -> printenv",
            [applets["xargs"], "-0", applets["env"], "BIG=" + payload, applets["printenv"], "BIG"],
            b"",
            payload.encode() + b"\n",
        ),
    ]
    for name, argv, input_bytes, expected in cases:
        if name.startswith("xargs -> env"):
            # Feed the oversized assignment through stdin so Python itself
            # does not hit Windows' native command-line limit.
            argv = [applets["xargs"], "-0", applets["env"]]
            input_bytes = ("BIG=" + payload + "\0" + applets["printenv"] + "\0BIG\0").encode()
        result = subprocess.run(argv, input=input_bytes, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        if result.returncode != 0 or result.stdout != expected:
            raise SystemExit(f"{name}: status={result.returncode}, output={len(result.stdout)} bytes, stderr={result.stderr[:300]!r}")
        print(f"PASS {name}: {len(result.stdout)} bytes")
