import contextlib
import importlib.util
import io
import json
from pathlib import Path
import shlex
import subprocess
import tarfile
import tempfile
import unittest
from unittest.mock import patch


ROOT = Path(__file__).resolve().parents[2]


def load(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


local = load("local_update", ROOT / "scripts/update-nas.py")
remote = load("remote_update", ROOT / "deploy/nas/update.py")


class NASUpdateTests(unittest.TestCase):
    def test_ssh_quotes_remote_paths_and_program_as_single_arguments(self):
        helper = "print('hello; $(whoami)')"
        path = "/a path/with'quotes;$(whoami)"
        with patch.object(local.subprocess, "run") as run:
            local.ssh("nas", helper, "apply", path, "digest")
        self.assertEqual(shlex.split(run.call_args.args[0][-1]), ["python3", "-c", helper, "apply", path, "digest"])

    def test_committed_export_excludes_ignored_mac_binary_and_secrets(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp) / "repo"
            root.mkdir()
            subprocess.run(["git", "init", "-q", str(root)], check=True)
            (root / ".gitignore").write_text("bin/go2rtc\nlocal.env\n")
            (root / "Makefile").write_text("release:\n\t@test ! -e bin/go2rtc\n\t@test ! -e local.env\n\t@test \"$(GOOS)/$(GOARCH)/$(CGO_ENABLED)\" = linux/amd64/0\n\t@mkdir -p .release\n\t@tar -czf .release/camera-appliance-latest.tar.gz Makefile\n")
            subprocess.run(["git", "add", "."], cwd=root, check=True)
            subprocess.run(["git", "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-qm", "test"], cwd=root, check=True)
            (root / "bin").mkdir()
            (root / "bin/go2rtc").write_text("Mac executable")
            (root / "local.env").write_text("private value")
            version, commit = local.release_identity(root, "dev")
            self.assertEqual(version, "dev-nas-" + commit)
            workspace = Path(temp) / "build"
            workspace.mkdir()
            self.assertTrue(local.build_release(root, workspace, version, commit, "amd64").is_file())
            (root / "Makefile").write_text("changed")
            with self.assertRaisesRegex(RuntimeError, "committen"):
                local.release_identity(root, "dev")

    def test_upload_digest_failure_cleans_staging_before_extracting(self):
        with tempfile.TemporaryDirectory() as temp:
            env = {"CAMERA_APPLIANCE_STATE_DIR": temp}
            stdin = io.TextIOWrapper(io.BytesIO(b"corrupt archive"))
            with patch.object(remote.sys, "stdin", stdin):
                with self.assertRaisesRegex(RuntimeError, "SHA-256"):
                    remote.upload(env, "incorrect")
            self.assertEqual(list((Path(temp) / "updates").iterdir()), [])

    def test_bootstrap_rejects_symlink_binary(self):
        with tempfile.TemporaryDirectory() as temp:
            archive = Path(temp) / "release.tar.gz"
            with tarfile.open(archive, "w:gz") as tar:
                member = tarfile.TarInfo("release/bin/camera-appliance")
                member.type = tarfile.SYMTYPE
                member.linkname = "/bin/sh"
                tar.addfile(member)
            with self.assertRaisesRegex(RuntimeError, "Binary"):
                remote.extract_worker(archive, Path(temp) / "worker")
            self.assertFalse((Path(temp) / "worker").exists())

    def test_stage_path_cannot_escape_updates_directory(self):
        with self.assertRaisesRegex(RuntimeError, "Staging"):
            remote.stage_directory({"CAMERA_APPLIANCE_STATE_DIR": "/data"}, "/data/updates/nas-upload-x/../../other")

    def test_obsolete_go_option_is_removed_only_for_appliance_units(self):
        with tempfile.TemporaryDirectory() as temp:
            with patch.object(remote.Path, "home", return_value=Path(temp)), patch.object(remote.subprocess, "check_output", return_value="GODEBUG=tlskyber=0,http2client=0\n"), patch.object(remote.subprocess, "run") as run:
                remote.compatible_go_environment()
            paths = list(Path(temp).rglob("*.conf"))
            self.assertEqual(len(paths), 2)
            for path in paths:
                self.assertIn('Environment="GODEBUG=http2client=0"', path.read_text())
                self.assertIn(path.parent.name, ("camera-appliance.service.d", "camera-appliance-update-.service.d"))
            run.assert_called_once_with(["systemctl", "--user", "daemon-reload"], check=True)

    def test_failed_or_replaced_job_never_reports_success(self):
        for phase, exit_code, job_id in (("failed", 1, "mine"), ("complete", 1, "mine"), ("complete", 0, "other")):
            with self.subTest(phase=phase, exit_code=exit_code, job_id=job_id), tempfile.TemporaryDirectory() as temp:
                stage = Path(temp)
                def run(args, **kwargs):
                    if args[-1] == "status":
                        return subprocess.CompletedProcess(args, 0, stdout=json.dumps({"id": job_id, "phase": phase}))
                    kwargs["stdout"].write('{"id":"mine","phase":"queued"}\n')
                    return subprocess.CompletedProcess(args, exit_code)
                with patch.object(remote, "compatible_go_environment"), patch.object(remote.subprocess, "run", side_effect=run), contextlib.redirect_stdout(io.StringIO()) as output:
                    with self.assertRaises(RuntimeError):
                        remote.run_update({"CAMERA_APPLIANCE_INSTALL_DIR": temp}, stage, "digest")
                self.assertNotIn("Installiert:", output.getvalue())
                self.assertTrue((stage / "update.log").exists())


if __name__ == "__main__":
    unittest.main()
