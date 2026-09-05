"""Disposable Linux update fixture. Run only with the documented Docker command.

The manager, GitHub catalog/download, archive installer and durable worker are
real. Only systemd and go2rtc are replaced by local process/HTTP adapters.
"""

import http.server
import http.client
import socketserver
import socket
import select
import subprocess
import os
import pathlib
import tarfile
import json
import threading
import time
import shutil
import sqlite3
import hashlib

if (
    os.environ.get("CAMERA_APPLIANCE_UI_UPDATE_LAB") != "1"
    or not pathlib.Path("/.dockerenv").exists()
):
    raise SystemExit("Run this fixture only in its disposable Docker container.")

ROOT = pathlib.Path("/appliance")
STATE = pathlib.Path("/state")
CONTROL = pathlib.Path("/control")
CONTROL.mkdir(exist_ok=True)
ENV = dict(
    os.environ,
    CAMERA_APPLIANCE_INSTALL_DIR=str(ROOT),
    CAMERA_APPLIANCE_STATE_DIR=str(STATE),
    CAMERA_APPLIANCE_CONFIG_DIR="/config",
    CAMERA_APPLIANCE_FRONTEND_DIST=str(ROOT / "frontend/dist"),
    CAMERA_APPLIANCE_COMPOSE_FILE=str(ROOT / "compose.yaml"),
    CAMERA_APPLIANCE_SLOTS_FILE=str(ROOT / "config/slots.yaml"),
    CAMERA_APPLIANCE_BIND_ADDR="127.0.0.1:8091",
    CAMERA_APPLIANCE_RESTART_STRATEGY="systemd",
    CAMERA_APPLIANCE_GO2RTC_URL="http://127.0.0.1:1984",
    HTTPS_PROXY="http://127.0.0.1:18081",
    HTTP_PROXY="http://127.0.0.1:18081",
    NO_PROXY="127.0.0.1,localhost",
    PATH="/control:" + os.environ["PATH"],
)
manager = None


class Server(socketserver.ThreadingMixIn, http.server.HTTPServer):
    daemon_threads = True
    allow_reuse_address = True


class Quiet(http.server.BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def reply(self, value, status=200):
        body = json.dumps(value).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)


def stop():
    global manager
    if manager and manager.poll() is None:
        manager.terminate()
        try:
            manager.wait(timeout=5)
        except subprocess.TimeoutExpired:
            manager.kill()
            manager.wait()


def start():
    global manager
    log = open("/control/manager.log", "ab", buffering=0)
    manager = subprocess.Popen(
        [str(ROOT / "bin/camera-appliance"), "serve"],
        cwd=ROOT,
        env=ENV,
        stdout=log,
        stderr=log,
        start_new_session=True,
    )
    time.sleep(0.4)
    if manager.poll() is not None:
        raise RuntimeError("manager exited")


def reset():
    stop()
    for p in [ROOT, STATE, pathlib.Path("/config")]:
        if p.exists():
            shutil.rmtree(p)
        p.mkdir()
    archive = pathlib.Path("/fixture/camera-appliance-0.3.0-c232ad4.tar.gz")
    if (
        hashlib.sha256(archive.read_bytes()).hexdigest()
        != "b44ecc0e15fa0e5a945294e293000c300140b7fb660061205cd190f26e3a6203"
    ):
        raise RuntimeError("Unexpected baseline release digest")
    with tarfile.open(archive) as t:
        prefix = t.getnames()[0].split("/")[0] + "/"
        for m in t.getmembers():
            if m.name.startswith(prefix):
                m.name = m.name[len(prefix) :]
                t.extract(m, ROOT, filter="data")
    (pathlib.Path("/config") / "local.env").write_text(
        "TAPO_CAMERA_PASSWORD=synthetic-camera-password\n"
    )
    (pathlib.Path("/config") / "snapshot-upload-password.json").write_text(
        '{"target":"synthetic","password":"synthetic-upload-password"}'
    )
    subprocess.run(
        [str(ROOT / "bin/camera-appliance"), "status"],
        cwd=ROOT,
        env=ENV,
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
        check=True,
    )
    db = sqlite3.connect(str(STATE / "state.db"))
    for k, v in {
        "auto_discover": "false",
        "watchdog.enabled": "false",
        "render_after_discovery": "false",
        "fixture.customer": "preserve-me",
        "snapshot.naming.fixture": '{"mode":"fixed","filename":"camera.jpg","directory":"customer"}',
        "snapshot.crop.fixture": '{"enabled":true,"x":10,"y":10,"width":80,"height":80}',
        "snapshot.schedule.fixture": '{"enabled":false,"interval_seconds":60}',
    }.items():
        db.execute(
            "INSERT OR REPLACE INTO settings VALUES(?,?,?)",
            (k, v, "2026-09-05T00:00:00Z"),
        )
    db.commit()
    db.close()
    for flag in CONTROL.glob("fail-*"):
        flag.unlink()
    start()


def fingerprint():
    db = sqlite3.connect("file:/state/state.db?mode=ro", uri=True)
    settings = db.execute(
        "SELECT key,value FROM settings WHERE key NOT LIKE 'watchdog.last_%' "
        "AND key NOT LIKE 'watchdog.next_%' ORDER BY key"
    ).fetchall()
    result = {
        "settings": hashlib.sha256(json.dumps(settings).encode()).hexdigest(),
        "slots": db.execute(
            "SELECT id,label,role,default_stream,required,sort_order FROM slots ORDER BY id"
        ).fetchall(),
        "config": {
            p.name: hashlib.sha256(p.read_bytes()).hexdigest()
            for p in pathlib.Path("/config").iterdir()
            if p.is_file()
        },
    }

    db.close()
    return result


class Control(Quiet):
    def do_POST(self):
        try:
            if self.path == "/reset":
                reset()
            elif self.path == "/restart":
                if (CONTROL / "fail-restart").exists() and json.loads(
                    (ROOT / "manifest.json").read_text()
                )["version"] == "0.4.0":
                    (CONTROL / "fail-restart").unlink()
                    raise RuntimeError("injected service restart failure")
                stop()
                time.sleep(0.5)
                start()
            elif self.path == "/fail-download":
                (CONTROL / "fail-download").touch()
            elif self.path == "/allow-download":
                (CONTROL / "fail-download").unlink(missing_ok=True)
            elif self.path == "/fail-restart":
                (CONTROL / "fail-restart").touch()
            elif self.path == "/fail-copy":
                shutil.rmtree(ROOT / "desktop")
                (ROOT / "desktop").write_text("synthetic conflicting file")
            else:
                raise RuntimeError("unknown control action")
            self.reply({"ok": True})
        except Exception as e:
            self.reply({"error": str(e)}, 500)

    def do_GET(self):
        if self.path == "/fingerprint":
            self.reply(fingerprint())
        elif self.path == "/job":
            p = STATE / "updates/job.json"
            j = json.loads(p.read_text()) if p.exists() else {}
            self.reply({k: j[k] for k in ["id", "phase", "error", "result"] if k in j})
        else:
            self.reply({"pid": manager.pid if manager else None})


class Reverse(Quiet):
    def relay(self):
        try:
            conn = http.client.HTTPConnection("127.0.0.1", 8091, timeout=35)
            data = self.rfile.read(int(self.headers.get("Content-Length", "0")))
            headers = {
                k: v
                for k, v in self.headers.items()
                if k.lower() not in ["connection", "accept-encoding"]
            }
            conn.request(self.command, self.path, data, headers)
            r = conn.getresponse()
            body = r.read()
            self.send_response(r.status)
            for k, v in r.getheaders():
                if k.lower() not in [
                    "transfer-encoding",
                    "connection",
                    "content-length",
                ]:
                    self.send_header(k, v)
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
            conn.close()
        except Exception:
            self.reply({"error": "manager restarting"}, 503)

    do_GET = relay
    do_POST = relay
    do_PUT = relay


class Go2RTC(Quiet):
    def do_GET(self):
        self.reply({})


class Proxy(Quiet):
    def do_CONNECT(self):
        if (CONTROL / "fail-download").exists():
            self.reply({"error": "injected download outage"}, 502)
            return
        host, port = self.path.rsplit(":", 1)
        try:
            peer = socket.create_connection((host, int(port)), timeout=20)
            self.send_response(200)
            self.end_headers()
            while True:
                ready, _, _ = select.select([self.connection, peer], [], [], 30)
                if not ready:
                    break
                for src in ready:
                    data = src.recv(65536)
                    if not data:
                        return
                    (peer if src is self.connection else self.connection).sendall(data)
        finally:
            if "peer" in locals():
                peer.close()


for name, content in {
    "systemd-run": """#!/usr/local/bin/python
import subprocess,sys,os
args=[a for a in sys.argv[1:] if not a.startswith('--user') and not a.startswith('--collect') and not a.startswith('--unit=')]
with open('/control/worker.log','ab',buffering=0) as log:subprocess.Popen(args,stdin=subprocess.DEVNULL,stdout=log,stderr=log,start_new_session=True)
""",
    "systemctl": """#!/usr/local/bin/python
import sys,urllib.request
args=[a for a in sys.argv[1:] if a!='--user']
if 'restart' in args and args[-1].removesuffix('.service')=='camera-appliance':
 try:urllib.request.urlopen(urllib.request.Request('http://127.0.0.1:8081/restart',method='POST'),timeout=10)
 except Exception as e:print(e);sys.exit(1)
elif 'is-active' in args:print('active')
""",
}.items():
    p = CONTROL / name
    p.write_text(content)
    p.chmod(0o755)
for port, handler in [(8081, Control), (1984, Go2RTC), (18081, Proxy)]:
    threading.Thread(
        target=Server(
            ("0.0.0.0" if port == 8081 else "127.0.0.1", port), handler
        ).serve_forever,
        daemon=True,
    ).start()
reset()
Server(("0.0.0.0", 8080), Reverse).serve_forever()
