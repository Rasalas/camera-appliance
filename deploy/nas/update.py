#!/usr/bin/env python3
"""Remote half of scripts/update-nas.py; transmitted over SSH, no installation needed."""

import hashlib
import json
import os
from pathlib import Path
import platform
import shlex
import shutil
import subprocess
import sys
import tarfile
import tempfile


def service_environment():
    value = subprocess.check_output(
        ["systemctl", "--user", "show", "camera-appliance", "-p", "Environment", "--value"],
        text=True,
    )
    env = dict(item.split("=", 1) for item in shlex.split(value))
    if env.get("CAMERA_APPLIANCE_RESTART_STRATEGY") != "systemd":
        raise RuntimeError("Der NAS-Dienst muss CAMERA_APPLIANCE_RESTART_STRATEGY=systemd setzen.")
    for key in ("INSTALL_DIR", "STATE_DIR", "CONFIG_DIR"):
        path = env.get("CAMERA_APPLIANCE_" + key, "")
        if not Path(path).is_absolute() or Path(path) == Path("/"):
            raise RuntimeError("Im NAS-Dienst fehlt ein absoluter Pfad für " + key + ".")
    root = Path(env["CAMERA_APPLIANCE_INSTALL_DIR"])
    env.setdefault("CAMERA_APPLIANCE_COMPOSE_FILE", str(root / "compose.yaml"))
    env.setdefault("CAMERA_APPLIANCE_SLOTS_FILE", str(Path(env["CAMERA_APPLIANCE_CONFIG_DIR"]) / "slots.yaml"))
    return env


def compatible_go_environment():
    # The NAS user manager may still export a Go option removed in Go 1.27.
    # Scope the correction to our units; leave other applications untouched.
    values = subprocess.check_output(["systemctl", "--user", "show-environment"], text=True)
    debug = next((line.split("=", 1)[1] for line in values.splitlines() if line.startswith("GODEBUG=")), "")
    filtered = ",".join(part for part in debug.split(",") if part.split("=", 1)[0] != "tlskyber")
    if filtered == debug:
        return
    assignment = ("GODEBUG=" + filtered).replace("\\", "\\\\").replace('"', '\\"').replace("%", "%%")
    content = '# Ignore the obsolete NAS session option tlskyber.\n[Service]\nEnvironment="' + assignment + '"\n'
    for unit in ("camera-appliance.service", "camera-appliance-update-.service"):
        directory = Path.home() / ".config/systemd/user" / (unit + ".d")
        directory.mkdir(parents=True, exist_ok=True)
        (directory / "90-nas-go-runtime.conf").write_text(content)
    subprocess.run(["systemctl", "--user", "daemon-reload"], check=True)


def stage_directory(env, value):
    parent = Path(env["CAMERA_APPLIANCE_STATE_DIR"]) / "updates"
    path = Path(value)
    if path.is_symlink() or path.parent.resolve() != parent.resolve() or not path.name.startswith("nas-upload-"):
        raise RuntimeError("Ungültiges NAS-Staging-Verzeichnis.")
    return path


def extract_worker(archive, target):
    # Extract only the regular manager binary to a fixed destination. Release
    # validation/extraction remains the responsibility of the actual updater.
    with tarfile.open(archive) as release:
        binaries = [item for item in release if item.name.endswith("/bin/camera-appliance")]
        if len(binaries) != 1 or not binaries[0].isfile() or binaries[0].size > 128 * 1024 * 1024:
            raise RuntimeError("Release enthält kein eindeutiges Manager-Binary.")
        with release.extractfile(binaries[0]) as source, target.open("wb") as output:
            shutil.copyfileobj(source, output)
    target.chmod(0o700)


def upload(env, digest):
    parent = Path(env["CAMERA_APPLIANCE_STATE_DIR"]) / "updates"
    parent.mkdir(parents=True, exist_ok=True)
    stage = Path(tempfile.mkdtemp(prefix="nas-upload-", dir=parent))
    try:
        archive = stage / "release.tar.gz"
        with archive.open("wb") as output:
            shutil.copyfileobj(sys.stdin.buffer, output)
        if hashlib.sha256(archive.read_bytes()).hexdigest() != digest:
            raise RuntimeError("SHA-256-Prüfung des übertragenen Archivs fehlgeschlagen.")
        extract_worker(archive, stage / "camera-appliance")
    except BaseException:
        shutil.rmtree(stage)
        raise
    print(json.dumps({"stage": str(stage)}))


def run_update(env, stage, digest):
    compatible_go_environment()
    runtime = os.environ.copy()
    runtime.update(env)
    # SSH sessions can carry the same obsolete setting as the user manager.
    if "GODEBUG" in runtime:
        runtime["GODEBUG"] = ",".join(p for p in runtime["GODEBUG"].split(",") if p.split("=", 1)[0] != "tlskyber")
    log = stage / "update.log"
    with log.open("w") as output:
        result = subprocess.run(
            [str(stage / "camera-appliance"), "update", "--archive", str(stage / "release.tar.gz"), "--digest", "sha256:" + digest],
            cwd=env["CAMERA_APPLIANCE_INSTALL_DIR"], env=runtime,
            stdout=output, stderr=subprocess.STDOUT,
        )
    # Use the public CLI view: job.json itself also contains private config.
    status = subprocess.run(
        [str(stage / "camera-appliance"), "update", "status"],
        env=runtime, capture_output=True, text=True, check=True,
    )
    job = json.loads(status.stdout)
    if result.returncode or job["phase"] != "complete":
        raise RuntimeError("Update fehlgeschlagen; Details unter " + str(log) + ". " + job.get("error", ""))
    submitted, _ = json.JSONDecoder().raw_decode(log.read_text().lstrip())
    if job["id"] != submitted["id"]:
        raise RuntimeError("Ein weiterer Auftrag hat den Update-Status ersetzt; " + str(log) + " prüfen.")
    installed = job["result"]
    print("Installiert: " + installed["new_version"]["version"] + " (" + installed["new_version"]["commit"] + ")")
    print("Versionsprüfung, go2rtc und Viewer erreichbar.")
    print("Backup: " + installed["backup_path"])
    print("Rollback: " + installed["rollback_dir"])
    shutil.rmtree(stage)


def main():
    os.umask(0o077)
    env = service_environment()
    action = sys.argv[1]
    if action == "check":
        subprocess.run(["systemctl", "--user", "is-active", "--quiet", "camera-appliance"], check=True)
        if not shutil.which("systemd-run"):
            raise RuntimeError("systemd-run fehlt auf der NAS.")
        arch = {"x86_64": "amd64", "aarch64": "arm64"}.get(platform.machine())
        if platform.system() != "Linux" or not arch:
            raise RuntimeError("Unterstützt werden Linux amd64 und arm64.")
        print(json.dumps({"arch": arch, "install_dir": env["CAMERA_APPLIANCE_INSTALL_DIR"]}))
    elif action == "upload":
        upload(env, sys.argv[2])
    elif action == "apply":
        run_update(env, stage_directory(env, sys.argv[2]), sys.argv[3])
    elif action == "status":
        runtime = os.environ.copy()
        runtime.update(env)
        subprocess.run([str(Path(env["CAMERA_APPLIANCE_INSTALL_DIR"]) / "bin/camera-appliance"), "update", "status"], env=runtime, check=True)
    else:
        raise RuntimeError("Unbekannter NAS-Update-Schritt.")


if __name__ == "__main__":
    try:
        main()
    except (RuntimeError, OSError, ValueError, subprocess.CalledProcessError) as error:
        print("NAS-Update fehlgeschlagen: " + str(error), file=sys.stderr)
        sys.exit(1)
