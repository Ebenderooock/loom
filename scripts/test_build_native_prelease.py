import importlib.util
import hashlib
import json
from pathlib import Path
import tarfile
import tempfile
import unittest
from unittest.mock import patch
import zipfile


spec = importlib.util.spec_from_file_location(
    "native_prelease", Path(__file__).with_name("build-native-prelease.py")
)
native = importlib.util.module_from_spec(spec)
spec.loader.exec_module(native)


class VersionTests(unittest.TestCase):
    def test_windows_candidate_cannot_enter_production_archives(self):
        root = Path(__file__).resolve().parent.parent
        candidate = (root / ".goreleaser.windows-updater-candidate.yaml").read_text()
        production = (root / ".goreleaser.yaml").read_text()
        self.assertIn("tags: [embed, loom_updater_windows_candidate]", candidate)
        self.assertIn("tags: [loom_updater_windows_candidate]", candidate)
        self.assertIn("UNVALIDATED", candidate)
        self.assertIn("release:\n  disable: true", candidate)
        self.assertNotIn("loom_updater_windows_candidate", production)

    def test_both_branch_spellings(self):
        for ref in ("prelease/1.2.3", "prerelease/1.2.3"):
            self.assertEqual(
                native.version_for(ref, "a" * 40, 1794866400),
                "1.2.3-prelease.20261116220000.gaaaaaaaaaaaa",
            )

    def test_invalid_ref_or_sha(self):
        for ref in ("main", "prelease/01.2.3", "prelease/1.2.3/extra",
                    "prelease/1.2.3;exit", "prelease/1.2.3 -X attacker=value",
                    "prelease/1.2.3\n"):
            with self.assertRaises(ValueError):
                native.version_for(ref, "a" * 40, 1794866400)
        with self.assertRaises(ValueError):
            native.version_for("prelease/1.2.3", "not-a-sha", 1794866400)

    def test_linker_metadata_rejects_argument_injection(self):
        values = ["1.2.3-prelease.20261116220000.gaaaaaaaaaaaa", "a" * 40,
                  "2026-11-16T22:00:00Z", "prelease/1.2.3"]
        flags = native.linker_flags(*values)
        self.assertIn(".SourceRef=refs/heads/prelease/1.2.3", flags)
        for i in range(len(values)):
            for injected in (" value", "'-X", "\n-X", "$(whoami)"):
                altered = values.copy()
                altered[i] += injected
                with self.subTest(field=i, injected=injected), self.assertRaises(ValueError):
                    native.linker_flags(*altered)

    def test_native_target_matrix(self):
        self.assertEqual(len(native.TARGETS), 7)
        self.assertIn(("windows", "arm64", ""), native.TARGETS)
        self.assertIn(("linux", "arm", "7"), native.TARGETS)

    def test_build_packages_targets_and_hashes_without_signing(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            for name in ("web/dist/index.html", "LICENSE", "README.md", "CHANGELOG.md",
                         "docs/deployment.md", "packaging/linux/install.sh",
                         "packaging/windows/install.ps1", "packaging/macos/install.sh"):
                path = root / name
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_text("test fixture\n")
            environments = []

            def fake_build(command, **kwargs):
                if command[-1]=="./cmd/loom":
                    environments.append(kwargs["env"])
                self.assertEqual(command[0:2], ["go", "build"])
                if command[-1]=="./cmd/loom":
                    self.assertIn("embed", command)
                    flags = command[command.index("-ldflags") + 1]
                    self.assertIn(".Version=1.2.3-prelease.20261116220000.gaaaaaaaaaaaa", flags)
                    self.assertIn(".Commit=" + "a" * 40, flags)
                    self.assertIn(".Date=2026-11-16T22:00:00Z", flags)
                    self.assertIn(".SourceRef=refs/heads/prelease/1.2.3", flags)
                Path(command[command.index("-o") + 1]).write_bytes(b"fixture binary")

            with patch.object(native, "ROOT", root), \
                 patch.object(native.subprocess, "check_output",
                              side_effect=["a" * 40, "1794866400"]), \
                 patch.object(native.subprocess, "run", side_effect=fake_build):
                output = root / "dist"
                native.build("prelease/1.2.3", output)
            inventory = json.loads((output / "native-artifacts.json").read_text())
            self.assertEqual(inventory["tag"], "v1.2.3-prelease.20261116220000.gaaaaaaaaaaaa")
            self.assertNotIn("signature", inventory)
            self.assertNotIn("expires", inventory)  # not part of the publisher contract
            self.assertEqual(len(inventory["artifacts"]), 7)
            self.assertEqual(len(list(output.iterdir())), 8)  # no signing key/signature
            for artifact, environment in zip(inventory["artifacts"], environments):
                archive = output / artifact["name"]
                data = archive.read_bytes()
                self.assertEqual(artifact["size"], len(data))
                self.assertEqual(artifact["sha256"], hashlib.sha256(data).hexdigest())
                self.assertEqual(environment["GOOS"], artifact["os"])
                self.assertEqual(environment["GOARCH"], artifact["arch"])
                self.assertEqual(environment.get("GOARM", ""), artifact.get("arm", ""))
                self.assertEqual(environment["CGO_ENABLED"], "0")
                if artifact["os"] == "windows":
                    with zipfile.ZipFile(archive) as zf:
                        names = zf.namelist()
                    self.assertIn("loom.exe", names)
                    self.assertNotIn("loom-updater.exe", names)
                    self.assertNotIn("loom-updater", names)
                    self.assertIn("packaging/windows/install.ps1", names)
                else:
                    with tarfile.open(archive) as tf:
                        names = tf.getnames()
                    self.assertIn("loom", names)
                    self.assertIn("loom-updater",names)
                    platform = "macos" if artifact["os"] == "darwin" else "linux"
                    self.assertIn(f"packaging/{platform}/install.sh", names)
                self.assertIn("docs/deployment.md", names)
                self.assertIn("LICENSE", names)
