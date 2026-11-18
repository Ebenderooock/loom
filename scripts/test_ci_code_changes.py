import importlib.util
from pathlib import Path
import subprocess
import unittest
from unittest.mock import patch


SPEC = importlib.util.spec_from_file_location(
    "ci_code_changes", Path(__file__).with_name("ci_code_changes.py")
)
filter_changes = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(filter_changes)


class CodeChangeFilterTests(unittest.TestCase):
    def test_docs_and_markdown_only_changes_are_not_code(self):
        for path in [
            "docs/api.md",
            "docs/images/diagram.png",
            "CHANGELOG.md",
            "README.md",
        ]:
            with self.subTest(path=path):
                self.assertFalse(filter_changes.is_code_change(path))

    def test_code_and_documentation_changes_run_validation(self):
        changes = ["CHANGELOG.md", "internal/server.go"]
        self.assertTrue(any(filter_changes.is_code_change(path) for path in changes))

    def test_non_documentation_changes_are_code(self):
        for path in [
            "internal/server/server.go",
            "web/src/App.tsx",
            ".github/workflows/ci.yml",
        ]:
            with self.subTest(path=path):
                self.assertTrue(filter_changes.is_code_change(path))

    @patch.object(filter_changes.subprocess, "run")
    def test_pull_request_diff_uses_merge_base(self, run):
        run.return_value = subprocess.CompletedProcess([], 0, "docs/api.md\n")
        self.assertEqual(
            filter_changes.changed_files(
                "pull_request", "", "", "base-sha", "head-sha"
            ),
            ["docs/api.md"],
        )
        self.assertEqual(
            run.call_args.args[0],
            ["git", "diff", "--name-only", "base-sha...head-sha"],
        )

    @patch.object(filter_changes.subprocess, "run")
    def test_push_diff_uses_before_and_after_shas(self, run):
        run.return_value = subprocess.CompletedProcess([], 0, "internal/server.go\n")
        self.assertEqual(
            filter_changes.changed_files(
                "push", "refs/heads/topic", "before-sha", "", "head-sha"
            ),
            ["internal/server.go"],
        )
        self.assertEqual(
            run.call_args.args[0],
            ["git", "diff", "--name-only", "before-sha", "head-sha"],
        )

    @patch.object(filter_changes.subprocess, "run")
    def test_new_pushes_and_release_tags_run_full_validation(self, run):
        self.assertIsNone(
            filter_changes.changed_files(
                "push", "refs/heads/topic", filter_changes.ZERO_SHA, "", "head-sha"
            )
        )
        self.assertIsNone(
            filter_changes.changed_files(
                "push", "refs/tags/v1.2.3", filter_changes.ZERO_SHA, "", "head-sha"
            )
        )
        run.assert_not_called()


if __name__ == "__main__":
    unittest.main()
