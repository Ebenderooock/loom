import importlib.util
import io
import json
from pathlib import Path
import unittest
from unittest.mock import patch


SPEC = importlib.util.spec_from_file_location(
    "wait_for_publisher", Path(__file__).with_name("wait-for-publisher.py")
)
observer = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(observer)


def publication(state="success", **overrides):
    return {
        "context": "loom-publisher/main",
        "creator": {"login": "loom-release[bot]"},
        "target_url": "https://github.com/Ebenderooock/loom-publisher/actions/runs/123",
        "state": state,
        **overrides,
    }


class PublisherObserverTests(unittest.TestCase):
    def test_only_trusts_the_publisher_app_for_the_exact_context(self):
        trusted = publication()
        statuses = [
            publication(creator={"login": "github-actions[bot]"}),
            publication(creator=None),
            publication(context="loom-publisher/prelease/0.5.0"),
            publication(
                target_url="https://github.com/attacker/loom-publisher/actions/runs/123"
            ),
            trusted,
        ]
        self.assertIs(
            observer.trusted_status(statuses, "loom-publisher/main", "loom-release"),
            trusted,
        )

    def test_pending_supersedes_an_older_success(self):
        pending = publication("pending")
        self.assertIs(
            observer.trusted_status(
                [pending, publication()], "loom-publisher/main", "loom-release"
            ),
            pending,
        )

    def test_unknown_publisher_state_fails_explicitly(self):
        with self.assertRaisesRegex(ValueError, "unknown publication state"):
            observer.trusted_status(
                [publication("unexpected")], "loom-publisher/main", "loom-release"
            )

    def test_rejects_lookalike_run_urls(self):
        for url in [
            "https://github.com.evil.test/Ebenderooock/loom-publisher/actions/runs/123",
            "https://github.com/Ebenderooock/loom-publisher/actions/runs/123/extra",
            "http://github.com/Ebenderooock/loom-publisher/actions/runs/123",
            "https://github.com/Ebenderooock/loom-publisher/actions/runs/not-a-run",
            None,
        ]:
            with self.subTest(url=url):
                self.assertIsNone(
                    observer.trusted_status(
                        [publication(target_url=url)],
                        "loom-publisher/main",
                        "loom-release",
                    )
                )

    @patch.object(observer.urllib.request, "urlopen")
    def test_checks_later_status_pages(self, urlopen):
        urlopen.side_effect = [
            io.BytesIO(json.dumps([publication(context="unrelated")] * 100).encode()),
            io.BytesIO(json.dumps([publication()]).encode()),
        ]
        result = observer.fetch_status(
            "a" * 40, "test-token", "loom-publisher/main", "loom-release"
        )
        self.assertEqual(result["state"], "success")
        self.assertIn("page=2", urlopen.call_args.args[0].full_url)

    @patch.object(observer.urllib.request, "urlopen")
    def test_rejects_invalid_api_response(self, urlopen):
        urlopen.return_value = io.BytesIO(b'{"message": "unexpected"}')
        with self.assertRaisesRegex(ValueError, "invalid commit-status"):
            observer.fetch_status(
                "a" * 40, "test-token", "loom-publisher/main", "loom-release"
            )

    @patch.object(observer, "fetch_status")
    def test_publishing_failure_fails_the_observer(self, fetch_status):
        for state in ["failure", "error"]:
            with self.subTest(state=state):
                fetch_status.return_value = publication(state)
                with self.assertRaisesRegex(ValueError, f"Publication {state}"):
                    observer.wait_for_publication(
                        "a" * 40, "test-token", "loom-publisher/main", "loom-release"
                    )

    @patch.object(observer.time, "sleep")
    @patch.object(observer, "fetch_status")
    def test_waits_until_actual_success(self, fetch_status, sleep):
        fetch_status.side_effect = [None, publication("pending"), publication()]
        observer.wait_for_publication(
            "a" * 40, "test-token", "loom-publisher/main", "loom-release"
        )
        self.assertEqual(fetch_status.call_count, 3)
        self.assertEqual(sleep.call_count, 2)

    @patch.object(observer.time, "sleep")
    @patch.object(observer, "fetch_status")
    def test_recovers_from_a_transient_api_failure(self, fetch_status, sleep):
        fetch_status.side_effect = [
            observer.urllib.error.URLError("temporary network failure"),
            publication(),
        ]
        result = observer.wait_for_publication(
            "a" * 40, "test-token", "loom-publisher/main", "loom-release"
        )
        self.assertEqual(result["state"], "success")
        sleep.assert_called_once()

    @patch.object(observer.time, "sleep")
    @patch.object(observer, "fetch_status")
    def test_authentication_errors_are_not_retried(self, fetch_status, sleep):
        fetch_status.side_effect = observer.urllib.error.HTTPError(
            "https://api.github.com", 401, "Unauthorized", {}, None
        )
        with self.assertRaises(observer.urllib.error.HTTPError):
            observer.wait_for_publication(
                "a" * 40, "test-token", "loom-publisher/main", "loom-release"
            )
        sleep.assert_not_called()

    @patch.object(observer.time, "sleep")
    @patch.object(observer, "fetch_status")
    def test_persistent_network_failure_is_not_reported_as_success(
        self, fetch_status, sleep
    ):
        fetch_status.side_effect = observer.urllib.error.URLError("network unavailable")
        with self.assertRaises(observer.urllib.error.URLError):
            observer.wait_for_publication(
                "a" * 40, "test-token", "loom-publisher/main", "loom-release"
            )
        self.assertEqual(fetch_status.call_count, 3)

    @patch.object(observer, "fetch_status", return_value=None)
    def test_timeout_is_not_reported_as_success(self, fetch_status):
        with self.assertRaisesRegex(TimeoutError, "not confirmed"):
            observer.wait_for_publication(
                "a" * 40, "test-token", "loom-publisher/main", "loom-release", timeout=0
            )

    @patch.dict(observer.os.environ, {"PUBLISHER_APP_SLUG": "", "GH_TOKEN": "test-token"})
    def test_missing_app_configuration_fails_explicitly(self):
        with self.assertRaisesRegex(ValueError, "PUBLISHER_APP_SLUG"):
            observer.main()


if __name__ == "__main__":
    unittest.main()
