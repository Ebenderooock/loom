import importlib.util
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch


SPEC = importlib.util.spec_from_file_location(
    "go_ci", Path(__file__).with_name("go-ci.py")
)
ci = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(ci)


class GoCITests(unittest.TestCase):
    def test_partition_covers_every_race_package_once_and_is_deterministic(self):
        workflows = {"loom/internal/workflows", "loom/internal/workflows/new"}
        race = {
            "loom/" + name for name in ci.WEIGHTS
        } | {
            "loom/cmd/loom", "loom/internal/new",
            "loom/internal/workflows_extra",
        }
        shards = ci.partition(race | workflows, workflows, 4)
        flattened = [package for shard in shards for package in shard]
        self.assertEqual(set(flattened), race)
        self.assertEqual(len(flattened), len(race))
        self.assertTrue(all(shards))
        self.assertEqual(
            shards, ci.partition(set(reversed(sorted(race | workflows))), workflows, 4)
        )
        # Spread the four slowest known packages across all four runners.
        slowest = {
            "loom/internal/" + name
            for name in ["collections", "requests", "downloads", "imports"]
        }
        self.assertTrue(all(len(set(shard) & slowest) == 1 for shard in shards))

    def test_rejects_invalid_partitions(self):
        for packages, workflows, count in [
            ({"a", "w"}, {"w"}, 0),
            ({"a"}, set(), 1),
            ({"a"}, {"w"}, 1),
            ({"a", "w"}, {"w"}, 2),
        ]:
            with self.subTest(packages=packages, workflows=workflows, count=count):
                with self.assertRaises(ValueError):
                    ci.partition(packages, workflows, count)

    @patch.object(ci.subprocess, "run")
    def test_discovery_uses_go_list_and_propagates_errors(self, run):
        run.side_effect = [
            subprocess.CompletedProcess([], 0, "loom/a\nloom/w\n"),
            subprocess.CompletedProcess([], 0, "loom/w\n"),
        ]
        self.assertEqual(ci.discover(1), [["loom/a"]])
        self.assertEqual(
            [call.args[0] for call in run.call_args_list],
            [
                ["go", "list", "-tags", "disable_libutp", "./..."],
                ["go", "list", "-tags", "disable_libutp", "./internal/workflows/..."],
            ],
        )
        run.side_effect = subprocess.CalledProcessError(1, ["go", "list"])
        with self.assertRaises(subprocess.CalledProcessError):
            ci.discover(1)
        run.side_effect = None
        for output in ["", "loom/a\nloom/a\n"]:
            run.return_value = subprocess.CompletedProcess([], 0, output)
            with self.assertRaises(ValueError):
                ci.go_list("./...")

    @patch.object(ci.subprocess, "run")
    def test_runner_forces_uncached_race_coverage_and_propagates_failure(self, run):
        ci.run_shard(["loom/a", "loom/b"], Path("coverage-0.out"))
        self.assertEqual(
            run.call_args.args[0],
            ["go", "test", "-race", "-tags", "disable_libutp", "-count=1",
             "-coverprofile=coverage-0.out", "loom/a", "loom/b"],
        )
        self.assertEqual(run.call_args.kwargs["env"]["CGO_ENABLED"], "1")
        self.assertEqual(run.call_args.kwargs["env"].get("PATH"), os.environ.get("PATH"))
        self.assertTrue(run.call_args.kwargs["check"])
        run.side_effect = subprocess.CalledProcessError(1, ["go", "test"])
        with self.assertRaises(subprocess.CalledProcessError):
            ci.run_shard(["loom/a"], Path("coverage-0.out"))
        run.reset_mock()
        with self.assertRaises(ValueError):
            ci.run_shard([], Path("coverage-0.out"))
        run.assert_not_called()

    def test_merges_atomic_profiles_with_exactly_one_header(self):
        with tempfile.TemporaryDirectory() as temp:
            directory = Path(temp)
            (directory / "coverage-0.out").write_text("mode: atomic\nb.go:1.1,2.1 1 4\n")
            (directory / "coverage-1.out").write_text("mode: atomic\na.go:1.1,2.1 2 0\n")
            output = directory / "combined.txt"
            ci.merge_coverage(directory, 2, output)
            self.assertEqual(
                output.read_text(),
                "mode: atomic\na.go:1.1,2.1 2 0\nb.go:1.1,2.1 1 4\n",
            )

    def test_rejects_missing_extra_mixed_or_overlapping_profiles(self):
        for second in [
            None, "mode: set\nb.go:1.1,2.1 1 0\n",
            "mode: atomic\na.go:1.1,2.1 1 0\n",
            "mode: atomic\ninvalid\n",
            "mode: atomic\nb.go:1.1,2.1 1 -1\n",
        ]:
            with self.subTest(second=second), tempfile.TemporaryDirectory() as temp:
                directory = Path(temp)
                (directory / "coverage-0.out").write_text(
                    "mode: atomic\na.go:1.1,2.1 1 1\n"
                )
                if second is not None:
                    (directory / "coverage-1.out").write_text(second)
                with self.assertRaises(ValueError):
                    ci.merge_coverage(directory, 2, directory / "combined.txt")
                self.assertFalse((directory / "combined.txt").exists())
        with tempfile.TemporaryDirectory() as temp:
            directory = Path(temp)
            (directory / "coverage-0.out").write_text("mode: atomic\n")
            with self.assertRaisesRegex(ValueError, "empty combined"):
                ci.merge_coverage(directory, 1, directory / "combined.txt")
            (directory / "coverage-1.out").write_text("mode: atomic\n")
            with self.assertRaisesRegex(ValueError, "unexpected"):
                ci.merge_coverage(directory, 1, directory / "combined.txt")


if __name__ == "__main__":
    unittest.main()
