import os
from pathlib import Path
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]


class RepairUpdateTests(unittest.TestCase):
    def fixture(self, root, curl_success=True, latest="https://github.com/Rasalas/camera-appliance/releases/tag/v9.9.9"):
        commands = root / "commands"
        commands.mkdir()
        install = root / "appliance"
        (install / "bin").mkdir(parents=True)
        (install / "bin/camera-appliance").write_text("#!/bin/sh\nexit 0\n")
        (install / "bin/camera-appliance").chmod(0o700)
        scripts = {
            "uname": "echo Linux\n",
            "id": "echo 0\n",
            "docker": "exit 0\n",
            "curl": "exit 22\n" if not curl_success else """for arg do
  case "$arg" in %{url_effective}) printf '%s' "$CAMERA_TEST_LATEST"; exit 0 ;; esac
  output="$arg"
done
for arg do case "$arg" in http*) echo "$arg" >> "$CAMERA_TEST_CURL_LOG" ;; esac; done
cat > "$output" <<'INSTALLER'
#!/bin/bash
printf '%s\\n' "$@" > "$CAMERA_TEST_REPAIR_ARGS"
INSTALLER
""",
        }
        for name, content in scripts.items():
            path = commands / name
            path.write_text("#!/bin/sh\n" + content)
            path.chmod(0o700)
        return {
            **os.environ,
            "PATH": str(commands) + os.pathsep + os.environ["PATH"],
            "CAMERA_APPLIANCE_INSTALL_DIR": str(install),
            "CAMERA_TEST_REPAIR_ARGS": str(root / "installer-args"),
            "CAMERA_TEST_CURL_LOG": str(root / "curl-log"),
            "CAMERA_TEST_LATEST": latest,
        }

    def test_existing_installation_uses_host_update_without_kiosk_changes(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            env = self.fixture(root)
            subprocess.run(["bash", str(ROOT / "repair-update.sh")], env=env, check=True, capture_output=True)
            args = (root / "installer-args").read_text().splitlines()
            self.assertEqual(args, [
                "--url", "https://github.com/Rasalas/camera-appliance/releases/download/v9.9.9/camera-appliance-latest.tar.gz",
                "--install-dir", env["CAMERA_APPLIANCE_INSTALL_DIR"],
                "--no-kiosk", "--no-desktop-launchers",
            ])
            self.assertEqual((root / "curl-log").read_text().splitlines(), [
                "https://raw.githubusercontent.com/Rasalas/camera-appliance/v9.9.9/install.sh",
            ])

    def test_unresolved_latest_release_never_executes_an_installer(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            env = self.fixture(root, latest="https://github.com/Rasalas/camera-appliance/releases")
            result = subprocess.run(["bash", str(ROOT / "repair-update.sh")], env=env, capture_output=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("Aktuelles Release konnte nicht ermittelt werden", result.stderr.decode())
            self.assertFalse((root / "installer-args").exists())

    def test_failed_download_never_executes_an_installer(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            env = self.fixture(root, curl_success=False)
            result = subprocess.run(["bash", str(ROOT / "repair-update.sh")], env=env, capture_output=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertFalse((root / "installer-args").exists())

    def test_missing_installation_never_becomes_a_fresh_install(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            env = self.fixture(root)
            env["CAMERA_APPLIANCE_INSTALL_DIR"] = str(root / "missing")
            result = subprocess.run(["bash", str(ROOT / "repair-update.sh")], env=env, capture_output=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("Bestehende Installation fehlt", result.stderr.decode())
            self.assertFalse((root / "installer-args").exists())


if __name__ == "__main__":
    unittest.main()
