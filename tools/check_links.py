#!/usr/bin/env python3
"""檢查本 fork 維護文件之間的相對連結。

只驗公開入口與 docs。外部網址交給人看。

    python tools/check_links.py
"""

from __future__ import annotations

import re
import sys
from pathlib import Path
from urllib.parse import unquote

ROOT = Path(__file__).resolve().parent.parent
LINK_PATTERN = re.compile(r"\[[^\]]*\]\(([^)]+)\)")
IMAGE_PATTERN = re.compile(r"!\[[^\]]*\]\(([^)]+)\)")
HTML_SRC_PATTERN = re.compile(r"""(?is)<img[^>]+src=["']([^"']+)["']""")
SKIP_PREFIXES = ("http://", "https://", "mailto:", "tel:", "#")
SKIP_NAMES = {
    "upstream-review-report.md",
    "dependency-freshness-report.md",
}
MAINTAINED_DOCUMENTS = (
    "README.md",
    "README.en.md",
    "FORK.md",
    "NOTICE.md",
    "AGENTS.md",
    "CONTRIBUTING.md",
    "SECURITY.md",
    "REVIEW.md",
    "docs/DEVELOPMENT.md",
    "docs/DECISIONS.md",
    "docs/UPSTREAM.md",
    ".github/pull_request_template.md",
)


def iter_documents() -> list[Path]:
    missing = [relative for relative in MAINTAINED_DOCUMENTS if not (ROOT / relative).is_file()]
    if missing:
        raise FileNotFoundError("maintained documents missing: " + ", ".join(missing))
    extra = []
    github = ROOT / ".github"
    if github.is_dir():
        extra.extend(
            path
            for path in github.rglob("*.md")
            if path.is_file() and path.name not in SKIP_NAMES
        )
    required = [ROOT / relative for relative in MAINTAINED_DOCUMENTS]
    seen = {path.resolve() for path in required}
    for path in extra:
        if path.resolve() not in seen:
            required.append(path)
            seen.add(path.resolve())
    return required


def _missing_relative(path: Path, target: str) -> str | None:
    target = target.strip().strip("<>")
    if not target or target.startswith(SKIP_PREFIXES):
        return None
    file_part = unquote(target.split("#", 1)[0])
    if not file_part:
        return None
    resolved = (path.parent / file_part).resolve()
    if resolved.exists():
        return None
    try:
        shown = resolved.relative_to(ROOT)
    except ValueError:
        shown = resolved
    return f"{target} → 找不到 {shown}"


def strip_code(text: str) -> str:
    text = re.sub(r"(?ms)<!--.*?-->", "", text)
    text = re.sub(r"(?ms)^```.*?^```", "", text)
    return re.sub(r"`[^`\n]+`", "", text)


def check_document(path: Path) -> list[str]:
    content = path.read_text(encoding="utf-8")
    stripped = strip_code(content)
    problems = []

    targets = LINK_PATTERN.findall(stripped)
    targets.extend(IMAGE_PATTERN.findall(stripped))
    targets.extend(HTML_SRC_PATTERN.findall(stripped))

    for target in targets:
        problem = _missing_relative(path, target)
        if problem:
            problems.append(problem)
    return problems


def main() -> int:
    total_problems = 0
    checked = 0
    for path in iter_documents():
        checked += 1
        problems = check_document(path)
        if problems:
            total_problems += len(problems)
            relative = path.relative_to(ROOT)
            print(f"FAIL: {relative}")
            for item in problems:
                print(f"  - {item}")
    if total_problems:
        print(f"\n{total_problems} broken relative links found across {checked} documents.")
        return 1
    print(f"OK: {checked} maintained documents have valid relative links.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
