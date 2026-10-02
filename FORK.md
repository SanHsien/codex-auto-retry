# Fork 維護說明

本 repo fork 自 [`sybxxx/codex-auto-retry`](https://github.com/sybxxx/codex-auto-retry)，
沿用 MIT License 與完整 Git 歷史。

## 為什麼維護 fork

- 專為 Windows 11 原生環境維護 Codex 自動重試與可靠性守護機制。
- 公開入口以繁體中文為主檔（`README.md`），英文鏡像放 `README.en.md`，原簡中說明保留為 `README.zh-CN.md`。
- 建立可重現的 Windows 開發 gate（`tools/dev_check.ps1`）、依賴新鮮度追蹤與上游水位檢查（涵蓋 commit、PR 與 issue）。
- 產品執行路徑以上游為準；Go 與 TypeScript 核心、Windows 托盤行程與安裝腳本完全保留。

**回貢判準：修的是上游的 bug 就送回去；這裡獨創的文件與 Windows 維護骨架留在這裡。**
回貢前必須在當次對話取得維護者明確同意；「fork」「建開發環境」「開 PR」都不是同意。

## 與上游的差異

| 項目 | 說明 |
|---|---|
| `README.md` | 繁中主檔；加入 fork 維護資訊與快速入口 |
| `README.en.md` | 英文鏡像；加入 fork 維護資訊 |
| `README.zh-CN.md` | 原簡體中文說明鏡像對照 |
| `AGENTS.md` | 本 fork 的 AI 維護單一真相源（純 Windows 維護線） |
| `CHANGELOG.md` / `CHANGELOG.en.md` | 本 fork 的維護歷史（Keep a Changelog 格式） |
| `NOTICE.md` / `FORK.md` / `LICENSE` | 來源、授權與同步說明 |
| `.cursor/rules/no-upstream-pr.mdc` | 防止誤向 upstream 開 PR 的機器層邊界防護 |
| `tools/dev_check.ps1` | Windows 本機一鍵 gate（語法、單元測試、相對連結檢查） |
| `tools/check_upstream_updates.py` | 上游 commit、PR 與 issue 水位檢查工具 |
| `tools/check_dependency_freshness.py` | 依賴新鮮度檢查工具 |
| `tools/check_links.py` | Markdown 相對連結完整性檢查工具 |
| `requirements-dev.txt` | Python 維護依賴清單（pytest、ruff） |
| `docs/DECISIONS.md`、`docs/UPSTREAM.md`、`docs/DEVELOPMENT.md` | fork 維護與決策文件 |
| `REVIEW.md` | 全庫風險快照 |

## 分支與 remote

- `origin/main`：SanHsien 維護線，也是唯一長期分支。
- `upstream/main`：原作者 `sybxxx/codex-auto-retry` 的主分支。

日常維護修改在本機跑過 `tools\dev_check.ps1` 後直接推 `origin/main`，不開功能分支。不推送到 `upstream`。
