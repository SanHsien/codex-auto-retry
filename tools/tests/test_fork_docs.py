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


def test_release_payload_entries_exist() -> None:
    script = (ROOT / "scripts" / "build-release.ps1").read_text(encoding="utf-8")
    match = re.search(r"foreach \(\$entry in @\(([^)]*)\)\)", script)
    assert match is not None
    entries = re.findall(r"'([^']+)'", match.group(1))
    assert "README.md" in entries
    assert "README.en.md" in entries
    missing = [name for name in entries if not (ROOT / name).exists()]
    assert missing == []


def test_fork_is_windows_only() -> None:
    source = ROOT / "scripts" / "source"
    assert sorted(path.name for path in source.glob("*_nonwindows.go")) == []
    documents = [*ROOT.glob("*.md"), *(ROOT / "docs").rglob("*.md")]
    stale = [
        str(path.relative_to(ROOT))
        for path in documents
        if "_nonwindows.go" in path.read_text(encoding="utf-8")
        and path.name not in {"CHANGELOG.md", "CHANGELOG.en.md", "DECISIONS.md", "REVIEW.md"}
        and path.name not in check_links.SKIP_NAMES
    ]
    assert stale == []
    assert not (ROOT / "README.zh-CN.md").exists()


# 上游產品字串出現過的簡體專用字；同步上游後若測試變紅，執行 tools/convert_zh_hant.py。
SIMPLIFIED_ONLY = set(
    "与专业两严个为义书买产仅从优会传体关内册冲况净击则删务动区协单历压参双发变号启员响围图处备复头夹实对将尝尽属带应"
    "开异弹强归当录彻径态总户执扫护拥择挡换据数断无旧时显暂机权条构标检气没测滚满点状独环现电监盖盘码确签简类紧纯线组"
    "终经结绕给绝统继绪续绿缓编缩网群脑脚节荐装见规览触计认让议记设证词试话询该语误说请读调败账购贵赖跃转轮软载较输达"
    "迁过运这进连适选递邮钟钥钮错键长闭问间队际随隐静顶项须预题额验骤"
)


def test_product_strings_are_traditional_chinese() -> None:
    import convert_zh_hant

    offenders = []
    for path in convert_zh_hant.iter_files():
        text = path.read_text(encoding="utf-8-sig")
        text = re.sub(r"\\u([0-9a-fA-F]{4})", lambda match: chr(int(match.group(1), 16)), text)
        for number, line in enumerate(text.splitlines(), 1):
            if any(marker in line for marker in convert_zh_hant.KEEP_LINE_MARKERS):
                continue
            found = SIMPLIFIED_ONLY.intersection(line)
            if found:
                offenders.append(f"{path.relative_to(ROOT)}:{number}: {''.join(sorted(found))}")
    assert offenders == []
    names = [path.name for path in (ROOT / "release" / "windows").iterdir()]
    assert [name for name in names if SIMPLIFIED_ONLY.intersection(name)] == []
    assert 'lang="zh-Hant-TW"' in (ROOT / "scripts/source/ui/panel.html").read_text(encoding="utf-8")


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
