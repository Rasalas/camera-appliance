#!/usr/bin/env python3
"""Build a committed release locally and invoke the native updater over SSH."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shlex
import subprocess
import sys
import tarfile
import tempfile


def ssh(host, helper, action, *args, **kwargs):
    # SSH joins remote arguments into a shell command; quote the entire Python
    # program and every argument, including paths returned by the NAS.
    command = shlex.join(["python3", "-c", helper, action, *args])
    return subprocess.run(
        ["ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=10", host, command],
        check=True, **kwargs,
    )


def release_identity(root, requested):
    dirty = subprocess.check_output(
        ["git", "status", "--porcelain", "--untracked-files=no"], cwd=root, text=True
    )
    if dirty.strip():
        raise RuntimeError("Änderungen zuerst committen; installiert wird ausschließlich HEAD.")
    commit = subprocess.check_output(
        ["git", "rev-parse", "--short", "HEAD"], cwd=root, text=True
    ).strip()
    version = requested if requested and requested != "dev" else "dev-nas-" + commit
    if not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9.+_-]{0,100}", version):
        raise RuntimeError("Ungültige VERSION.")
    return version, commit


def build_release(root, workspace, version, commit, arch):
    # Export tracked files into an empty directory: ignored local credentials,
    # data and native Mac helper binaries must never enter the NAS release.
    source = workspace / "source.tar"
    subprocess.run(["git", "archive", "--output", str(source), "HEAD"], cwd=root, check=True)
    build = workspace / "build"
    build.mkdir()
    subprocess.run(["tar", "-xf", str(source), "-C", str(build)], check=True)
    env = os.environ.copy()
    env.update(GOOS="linux", GOARCH=arch, CGO_ENABLED="0")
    # Recursive make flags from the caller can contain local paths or overrides.
    for key in ("MAKEFLAGS", "MFLAGS", "MAKEOVERRIDES"):
        env.pop(key, None)
    subprocess.run(
        ["make", "release", "VERSION=" + version, "COMMIT=" + commit, "RELEASE_DIR=" + str(build / ".release")],
        cwd=build, env=env, check=True,
    )
    archive = build / ".release/camera-appliance-latest.tar.gz"
    with tarfile.open(archive) as release:
        names = release.getnames()
        if any(name.endswith("/bin/go2rtc") for name in names):
            raise RuntimeError("Release enthält einen lokalen go2rtc-Helper.")
    return archive


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--status", action="store_true")
    args = parser.parse_args()
    host = os.environ.get("NAS_HOST", "nas")
    if not host or host.startswith("-") or any(c.isspace() for c in host):
        raise RuntimeError("NAS_HOST muss ein SSH-Alias oder user@host sein.")
    root = Path(__file__).resolve().parent.parent
    helper = (root / "deploy/nas/update.py").read_text()
    if args.status:
        ssh(host, helper, "status")
        return
    version, commit = release_identity(root, os.environ.get("NAS_VERSION"))
    result = ssh(host, helper, "check", stdout=subprocess.PIPE, text=True)
    target = json.loads(result.stdout)
    print("NAS erreichbar: Linux/" + target["arch"] + ", " + target["install_dir"], flush=True)
    with tempfile.TemporaryDirectory(prefix="camera-appliance-nas-") as temp:
        archive = build_release(root, Path(temp), version, commit, target["arch"])
        digest = hashlib.sha256(archive.read_bytes()).hexdigest()
        with archive.open("rb") as source:
            result = ssh(host, helper, "upload", digest, stdin=source, stdout=subprocess.PIPE)
        stage = json.loads(result.stdout)["stage"]
        print("Archiv geprüft; starte " + version + " (" + commit + ").", flush=True)
        try:
            ssh(host, helper, "apply", stage, digest)
        except (subprocess.CalledProcessError, KeyboardInterrupt):
            print("Update-Status prüfen mit: make update-nas-status NAS_HOST=" + shlex.quote(host), file=sys.stderr)
            print("NAS-Staging für Diagnose: " + stage, file=sys.stderr)
            raise


if __name__ == "__main__":
    try:
        main()
    except subprocess.CalledProcessError as error:
        print("NAS-Update fehlgeschlagen; Prozess-Exitcode " + str(error.returncode) + ".", file=sys.stderr)
        sys.exit(1)
    except (RuntimeError, OSError) as error:
        print("NAS-Update fehlgeschlagen: " + str(error), file=sys.stderr)
        sys.exit(1)
