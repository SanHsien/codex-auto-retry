#!/usr/bin/env python3
"""把上游簡體中文的產品字串轉成臺灣繁體中文。

同步上游後執行；需要額外安裝 OpenCC（`pip install opencc`），不屬於日常 gate 的依賴。

    python tools/convert_zh_hant.py            # 轉換並寫回
    python tools/convert_zh_hant.py --check    # 只列出會變動的檔案

BOM 與換行原樣保留。`\\uXXXX` 跳脫字串一併轉換；PowerShell 的 `[char]0xXXXX`
字元碼不處理，需人工對照。
"""

from __future__ import annotations

import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
TARGETS = (
    ".codex-plugin",
    "release/windows",
    "scripts",
    "skills",
    "docs/architecture.md",
    "docs/project-map.md",
    "docs/shared-backend-safety.md",
    "docs/releases",
    "assets/README.md",
    "README.en.md",
)
SUFFIXES = {".go", ".ps1", ".ts", ".html", ".css", ".md", ".txt", ".cmd", ".vbs", ".json"}
SKIP_PARTS = {"node_modules", "dist", "bin"}
SKIP_NAMES = {"package-lock.json"}
# 這兩個字串是「UTF-8 被當成 GBK 讀」的亂碼測試資料，必須逐字保留。
KEEP_LINE_MARKERS = ("缁х画",)

# OpenCC 之前先換掉的詞（簡體原詞 → 臺灣用語）。
PRE = (
    ("启动管理器", "啟動管理員"),
    ("卸载", "解除安裝"),
    ("共享后台", "共用後端"),
    ("官方后台", "官方後端"),
    ("后台进程", "背景行程"),
    ("后台服务", "背景服務"),
    ("后台", "後端"),
    ("进程", "行程"),
    ("托盘", "系統匣"),
    ("刷新", "重新整理"),
    ("退出", "結束"),
    ("点击", "按一下"),
    ("回环", "迴路"),
    ("可执行文件", "執行檔"),
    ("简体中文", "繁體中文"),
    ("登录", "登入"),
    ("配置", "設定"),
    ("首选", "偏好"),
    ("插件", "外掛"),
    ("回滚", "回復"),
    ("兼容", "相容"),
    ("窗口", "視窗"),
    ("界面", "介面"),
    ("激进", "激進"),
)
# OpenCC 之後再修的詞。
POST = (
    ("啟動管理器", "啟動管理員"),
    ("共享", "共用"),
    ("倒計時", "倒數計時"),
    ("當前", "目前"),
    ("禁用", "停用"),
    ("超時", "逾時"),
    ("型別", "類型"),
    ("校驗", "驗證"),
    ("未透過", "未通過"),
    ("顯式", "明確"),
    ("握手", "交握"),
    ("重啟", "重新啟動"),
    ("檢測到", "偵測到"),
    ("佈局", "版面"),
    ("透過健康檢查", "通過健康檢查"),
    ("健康檢查透過", "健康檢查通過"),
    ("許可權", "權限"),
    ("登入檔", "登錄檔"),
    ("登入機碼", "登錄機碼"),
    ("全域性", "全域"),
    ("二進位制", "二進位"),
    ("驗證和", "雜湊值"),
    ("賬號", "帳號"),
    ("未知釋出者", "未知發行者"),
    ("釋出", "發佈"),
    ("實時", "即時"),
    ("自定義", "自訂"),
    ("死迴圈", "無窮迴圈"),
    ("純綠色安裝", "只寫入目前使用者的資料夾"),
    ("64 位系統", "64 位元系統"),
    ("壓縮包", "壓縮檔"),
    ("解壓", "解壓縮"),
    ("解壓縮縮", "解壓縮"),
    ("控制檯", "主控台"),
    ("撥出", "叫出"),
    ("構建", "建置"),
    ("常規", "一般"),
    ("引數", "參數"),
    ("簽名證書", "簽章憑證"),
    ("外掛列表", "外掛清單"),
    ("所有者", "擁有者"),
    ("其他地址", "其他位址"),
    ("預裝", "預先安裝"),
    ("氣泡提醒", "通知"),
    ("啟動專案", "啟動項目"),
    ("隻影響", "只影響"),
    ("“", "「"),
    ("”", "」"),
)

# Characters OpenCC maps although they are also standard in Taiwan usage.
SHARED_FORMS = set("准台里后面干只系制征向并采表范几云价松")

ESCAPE_RUN = re.compile(r"(?:\\u[0-9a-fA-F]{4})+")
CJK = re.compile(r"[一-鿿]")


def build_converter():
    import opencc

    engine = opencc.OpenCC("s2twp")
    characters = opencc.OpenCC("s2t")

    def has_simplified(text: str) -> bool:
        # Phrase tables also rewrite valid Traditional text (登錄→登入,
        # 通過→透過), so only lines that really contain Simplified
        # characters are converted.
        return any(characters.convert(char) != char for char in text if CJK.match(char) and char not in SHARED_FORMS)

    def convert(text: str) -> str:
        if not has_simplified(text):
            return text
        for source, target in PRE:
            text = text.replace(source, target)
        text = engine.convert(text)
        for source, target in POST:
            text = text.replace(source, target)
        return text

    return convert


def convert_line(line: str, convert) -> str:
    if any(marker in line for marker in KEEP_LINE_MARKERS):
        return line

    def escaped(match: re.Match[str]) -> str:
        decoded = "".join(chr(int(code, 16)) for code in re.findall(r"\\u([0-9a-fA-F]{4})", match.group(0)))
        return "".join(f"\\u{ord(char):04x}" for char in convert(decoded))

    line = ESCAPE_RUN.sub(escaped, line)
    if CJK.search(line):
        line = convert(line)
    return line


def iter_files() -> list[Path]:
    found: list[Path] = []
    for target in TARGETS:
        path = ROOT / target
        candidates = [path] if path.is_file() else sorted(path.rglob("*")) if path.is_dir() else []
        for candidate in candidates:
            if not candidate.is_file() or candidate.suffix.lower() not in SUFFIXES:
                continue
            if candidate.name in SKIP_NAMES or SKIP_PARTS & set(candidate.relative_to(ROOT).parts):
                continue
            found.append(candidate)
    return found


def main() -> int:
    check_only = "--check" in sys.argv[1:]
    convert = build_converter()
    changed = 0
    for path in iter_files():
        raw = path.read_bytes()
        bom = raw.startswith(b"\xef\xbb\xbf")
        try:
            text = raw[3:].decode("utf-8") if bom else raw.decode("utf-8")
        except UnicodeDecodeError:
            continue
        lines = text.splitlines(keepends=True)
        result = "".join(convert_line(line, convert) for line in lines)
        if result == text:
            continue
        changed += 1
        print(path.relative_to(ROOT))
        if not check_only:
            path.write_bytes((b"\xef\xbb\xbf" if bom else b"") + result.encode("utf-8"))
    print(f"{changed} file(s) {'would change' if check_only else 'converted'}.")
    return 1 if check_only and changed else 0


if __name__ == "__main__":
    sys.exit(main())
