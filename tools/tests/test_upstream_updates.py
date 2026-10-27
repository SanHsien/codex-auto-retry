"""What the upstream checker must not be allowed to do.

Every test here names a way the check could go quiet without anybody deciding
to stop watching upstream.
"""

from __future__ import annotations

import json
import subprocess
import sys
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(REPO_ROOT / "tools"))

import check_upstream_updates as checker

BASELINE = {
    "repo": "https://github.com/example/product.git",
    "branch": "main",
    "reviewed_through": "a" * 40,
    "reviewed_date": "2026-10-02",
}


def fake_gh(payload: object, returncode: int = 0):
    """Stand in for `gh`, so no test reaches the network."""

    def runner(args, **kwargs):
        assert args[0] == "gh"
        assert "--state" in args and args[args.index("--state") + 1] == "all"
        return subprocess.CompletedProcess(
            args, returncode, stdout=json.dumps(payload), stderr=""
        )

    return runner


def test_ticket_titles_survive_undecodable_bytes(monkeypatch):
    """Titles are written by strangers and the console is not always UTF-8."""
    captured = {}

    def runner(args, **kwargs):
        captured.update(kwargs)
        return subprocess.CompletedProcess(args, 0, stdout="[]", stderr="")

    monkeypatch.setattr(checker.subprocess, "run", runner)
    checker.collect_new_tickets(BASELINE, "pr")

    assert captured["errors"] == "replace"


def test_gh_failure_reports_unchecked_rather_than_empty(monkeypatch):
    """`None`, not `[]`: "not checked" must never render as "nothing to review"."""
    monkeypatch.setattr(checker.subprocess, "run", fake_gh([], returncode=1))
    assert checker.collect_new_tickets(BASELINE, "issue") is None


def test_gh_missing_executable_reports_none(monkeypatch):
    """When `gh` executable is not installed or raises OSError, return None instead of crashing."""
    def missing_runner(*args, **kwargs):
        raise FileNotFoundError("No such file or directory: 'gh'")

    monkeypatch.setattr(checker.subprocess, "run", missing_runner)
    assert checker.collect_new_tickets(BASELINE, "issue") is None


def test_untracked_repo_url_refuses_to_guess_upstream_slug():
    assert checker.upstream_slug("https://gitlab.com/owner/name.git") is None
    assert (
        checker.upstream_slug("https://github.com/sybxxx/codex-auto-retry.git")
        == "sybxxx/codex-auto-retry"
    )
