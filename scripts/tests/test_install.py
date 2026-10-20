import gzip
import hashlib
import io
import os
from pathlib import Path
import subprocess
import tarfile
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]
RELEASE_URL = "https://example.test/releases/download/v9.9.9/camera-appliance-latest.tar.gz"

# Serves $CAMERA_TEST_SERVE/<basename of URL>; unknown files fail like curl -f.
CURL_STUB = """#!/bin/sh
output=""
url=""
while [ $# -gt 0 ]; do
  case "$1" in
    -o) output="$2"; shift 2 ;;
    -*) shift ;;
    *) url="$1"; shift ;;
  esac
done
echo "$url" >> "$CAMERA_TEST_LOG/curl"
file="$CAMERA_TEST_SERVE/${url##*/}"
[ -f "$file" ] || exit 22
cp "$file" "$output"
"""


def release_archive():
    binary = b"#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$CAMERA_TEST_LOG/release-binary\"\n"
    files = {
        "camera-appliance-9.9.9-abc/bin/camera-appliance": (binary, 0o755),
        "camera-appliance-9.9.9-abc/compose.yaml": (b"services: {}\n", 0o644),
        "camera-appliance-9.9.9-abc/release.env": (b"VERSION=9.9.9\n", 0o644),
    }
    buffer = io.BytesIO()
    # Fixed gzip mtime keeps the digest stable across calls.
    with gzip.GzipFile(fileobj=buffer, mode="wb", mtime=0) as compressed:
        with tarfile.open(fileobj=compressed, mode="w") as archive:
            for name, (data, mode) in files.items():
                info = tarfile.TarInfo(name)
                info.size, info.mode = len(data), mode
                archive.addfile(info, io.BytesIO(data))
    return buffer.getvalue()


class InstallScriptTests(unittest.TestCase):
    def run_install(self, root, sums=None, extra_args=(), compose_available=True, compose_sums=None):
        commands, serve, log = root / "commands", root / "serve", root / "log"
        for directory in (commands, serve, log):
            directory.mkdir()
        archive = release_archive()
        (serve / "camera-appliance-latest.tar.gz").write_bytes(archive)
        if sums is not None:
            (serve / "SHA256SUMS").write_text(sums(archive))
        if compose_sums is not None:
            (serve / "docker-compose-linux-x86_64").write_bytes(b"compose")
            (serve / "checksums.txt").write_text(compose_sums)
        docker = 'echo "$*" >> "$CAMERA_TEST_LOG/docker"\n'
        if not compose_available:
            docker += '[ "$1 $2" = "compose version" ] && exit 1\n'
        scripts = {
            "id": "echo 0\n",
            "uname": '[ "$1" = -m ] && echo x86_64 || echo Linux\n',
            "curl": CURL_STUB.split("\n", 1)[1],
            "docker": docker + "exit 0\n",
            "ffmpeg": "exit 0\n",
            "apt-get": "exit 0\n",
            "systemctl": "exit 0\n",
        }
        for name, content in scripts.items():
            path = commands / name
            path.write_text("#!/bin/sh\n" + content)
            path.chmod(0o700)
        env = {
            **os.environ,
            "PATH": str(commands) + os.pathsep + os.environ["PATH"],
            "CAMERA_TEST_SERVE": str(serve),
            "CAMERA_TEST_LOG": str(log),
        }
        args = ["bash", str(ROOT / "install.sh"), "--url", RELEASE_URL, "--install-dir", str(root / "appliance"),
                "--no-kiosk", "--no-desktop-launchers", *extra_args]
        return subprocess.run(args, env=env, capture_output=True, text=True), log

    @staticmethod
    def digest(data):
        return hashlib.sha256(data).hexdigest()

    def test_fresh_install_verifies_archive_and_builds_images_first(self):
        with tempfile.TemporaryDirectory() as temp:
            result, log = self.run_install(Path(temp), sums=lambda a: f"{self.digest(a)}  camera-appliance-latest.tar.gz\n")
            self.assertEqual(result.returncode, 0, result.stderr)
            args = (log / "release-binary").read_text().splitlines()
            self.assertEqual(args[:2], ["install", "--archive"])
            self.assertIn("sha256:" + self.digest(release_archive()), args)
            builds = [line for line in (log / "docker").read_text().splitlines() if line.endswith(" build")]
            self.assertEqual(len(builds), 1)
            self.assertIn("--project-name camera-appliance", builds[0])
            self.assertIn(
                "https://example.test/releases/download/v9.9.9/SHA256SUMS",
                (log / "curl").read_text().splitlines(),
            )

    def test_existing_installation_updates_with_verified_digest_without_rebuild(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            installed = root / "appliance/bin/camera-appliance"
            installed.parent.mkdir(parents=True)
            installed.write_text("#!/bin/sh\n")
            installed.chmod(0o700)
            result, log = self.run_install(root, sums=lambda a: f"{self.digest(a)}  camera-appliance-latest.tar.gz\n")
            self.assertEqual(result.returncode, 0, result.stderr)
            args = (log / "release-binary").read_text().splitlines()
            self.assertEqual(args[:2], ["update", "--archive"])
            self.assertIn("sha256:" + self.digest(release_archive()), args)
            self.assertNotIn(" build", (log / "docker").read_text())

    def test_mismatching_checksum_never_runs_the_release(self):
        with tempfile.TemporaryDirectory() as temp:
            result, log = self.run_install(Path(temp), sums=lambda a: f"{'0' * 64}  camera-appliance-latest.tar.gz\n")
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("SHA-256-Prüfung für camera-appliance-latest.tar.gz fehlgeschlagen", result.stderr)
            self.assertFalse((log / "release-binary").exists())
            self.assertFalse((log / "docker").exists() and " build" in (log / "docker").read_text())

    def test_archive_missing_from_checksums_is_refused(self):
        with tempfile.TemporaryDirectory() as temp:
            result, log = self.run_install(Path(temp), sums=lambda a: f"{self.digest(a)}  other.tar.gz\n")
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("Keine gültige SHA-256-Prüfsumme", result.stderr)
            self.assertFalse((log / "release-binary").exists())

    def test_release_without_checksums_is_refused(self):
        with tempfile.TemporaryDirectory() as temp:
            result, log = self.run_install(Path(temp))
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("SHA256SUMS konnte nicht geladen werden", result.stderr)
            self.assertFalse((log / "release-binary").exists())

    def test_explicit_sha256_replaces_published_checksums(self):
        with tempfile.TemporaryDirectory() as temp:
            expected = self.digest(release_archive()).upper()
            result, log = self.run_install(Path(temp), extra_args=("--sha256", expected))
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertTrue((log / "release-binary").exists())

    def test_compose_fallback_verifies_before_installing_plugin(self):
        for name, compose_sums, message in (
            ("checksums unavailable", None, "checksums.txt für Compose"),
            ("checksum mismatch", f"{'0' * 64} *docker-compose-linux-x86_64\n", "SHA-256-Prüfung für docker-compose-linux-x86_64"),
        ):
            with self.subTest(name), tempfile.TemporaryDirectory() as temp:
                result, log = self.run_install(
                    Path(temp),
                    sums=lambda a: f"{self.digest(a)}  camera-appliance-latest.tar.gz\n",
                    compose_available=False,
                    compose_sums=compose_sums,
                )
                self.assertNotEqual(result.returncode, 0)
                self.assertNotIn("unbound variable", result.stderr)
                self.assertIn(message, result.stderr)
                self.assertNotIn("docker-compose-linux-x86_64", (log / "curl").read_text().split("checksums.txt")[0])
                self.assertFalse((log / "release-binary").exists())


if __name__ == "__main__":
    unittest.main()
