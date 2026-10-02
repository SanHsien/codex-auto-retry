from __future__ import annotations

import re
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

import check_links
import check_upstream_updates as checker

ROOT = Path(__file__).resolve().parents[2]


def test_maintainer_markdown_links_resolve() -> None:
    failures = 0
    for path in check_links.iter_documents():
        problems = check_links.check_document(path)
        failures += len(problems)
        for problem in problems:
            print(f"{path}: {problem}")
    assert failures == 0


def test_required_overlay_files_exist() -> None:
    required = (
        "README.md",
        "README.en.md",
        "README.zh-CN.md",
        "FORK.md",
        "NOTICE.md",
        "AGENTS.md",
        "CHANGELOG.md",
        "CHANGELOG.en.md",
        "CONTRIBUTING.md",
        "SECURITY.md",
        "REVIEW.md",
        "docs/DEVELOPMENT.md",
        "docs/DECISIONS.md",
        "docs/UPSTREAM.md",
        "tools/dev_check.ps1",
        "tools/check_upstream_updates.py",
        "tools/check_dependency_freshness.py",
        "tools/check_links.py",
        "tools/upstream_baseline.json",
        "requirements-dev.txt",
        "LICENSE",
    )
    missing = [name for name in required if not (ROOT / name).is_file()]
    assert missing == []


def test_readme_pair_cross_links_and_names_the_fork() -> None:
    zh = (ROOT / "README.md").read_text(encoding="utf-8")
    en = (ROOT / "README.en.md").read_text(encoding="utf-8")
    assert "README.en.md" in zh
    assert "README.md" in en
    assert "sybxxx/codex-auto-retry" in zh
    assert "sybxxx/codex-auto-retry" in en
    assert "FORK.md" in zh
    assert "MIT" in zh


def test_gitignore_covers_user_data_and_reports() -> None:
    text = (ROOT / ".gitignore").read_text(encoding="utf-8")
    assert ".env" in text
    assert ".venv" in text
    assert "upstream-review-report.md" in text
    assert "dependency-freshness-report.md" in text


def test_review_snapshot_has_required_sections() -> None:
    text = (ROOT / "REVIEW.md").read_text(encoding="utf-8")
    assert "## 結論" in text
    assert "## 已修 findings" in text
    assert "## 接受、不改契約" in text
    assert "## 尚未宣稱範圍" in text


def test_baseline_file_is_valid_and_complete() -> None:
    baseline = checker.load_baseline()
    assert baseline["repo"] == "https://github.com/sybxxx/codex-auto-retry.git"
    assert baseline["branch"] == "main"
    assert len(baseline["reviewed_through"]) == 40
    assert baseline["reviewed_through"] == "867da682d1863c4d4eb142aab3078aca4df8e0f3"
    assert re.fullmatch(r"\d{4}-\d{2}-\d{2}", baseline["reviewed_date"])
    assert isinstance(baseline["reviewed_pr_through"], int)
    assert isinstance(baseline["reviewed_issue_through"], int)
