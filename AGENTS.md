# AGENTS.md

給 Codex、Claude Code、Cursor、Antigravity 與其他自動化代理在本專案工作時的指引。產品與使用方式先讀 [`README.md`](README.md)；開發與驗收細節見 [`docs/DEVELOPMENT.md`](docs/DEVELOPMENT.md)。

## 專案定位

這是 [`sybxxx/codex-auto-retry`](https://github.com/sybxxx/codex-auto-retry) 的 MIT License fork。
核心功能是專為 Windows 平台 Codex Desktop 打造的開源可靠性守護與原地自動接續工具，在遭遇服務限流（Rate Limit / 5 小時額度重設）、請求逾時或連線中斷時安全續跑原任務。

`origin` 是 `SanHsien/codex-auto-retry`（預設分支 `main`），`upstream` 是原作者 repo（預設分支 `main`）。
保留上游作者 `sybxxx` (TQY Local Tools)、MIT License 與產品程式。本 fork 的維護差異記在 [`FORK.md`](FORK.md) 與 [`docs/DECISIONS.md`](docs/DECISIONS.md)。

主要開發與完整驗收環境是 **Windows 11 + PowerShell**。本 fork 為純 Windows 維護線，所有測試與工作流程均在 Windows 原生環境執行。

## 硬性邊界

- 不提交使用者輸入檔案、專有文件、API key、token、私鑰或 `.env`。
- 不推送到 `upstream`。上游同步先跑 `python tools/check_upstream_updates.py`，逐筆審查後再 merge / cherry-pick；不盲目覆蓋 fork 文件與 Windows gate。
- 維護環境（`requirements-dev.txt`）僅安裝 pytest 與 ruff。
- 不把 fork 包裝成原創產品，不移除原作者 sybxxx (TQY Local Tools) 或官方連結。

## 技術與資料流

- 核心守護程式：`scripts/source/`（Go 撰寫，包含 RPC 命名管道、分類器、監護服務）。
- 內嵌管理面板：`scripts/source/ui/`（純 TypeScript，打包後由 Go embed 嵌入二進位檔）。
- 產品二進位檔：`scripts/bin/codex-auto-retry.exe` 與 `codex-auto-retry-mcp.exe`。
- Windows 部署與控制器：`release/windows/`（`deploy.ps1`、`startup-manager.ps1`、`uninstall-release.ps1`、`.cmd` 入口腳本）。
- 冒煙測試與驗證：`scripts/`（包含 `smoke-test.ps1`、`mcp-smoke-test.ps1` 等 PowerShell 測試）。
- 維護工具：`tools/`（Windows gate、上游檢查、相對連結檢查、依賴新鮮度）。
- 維護契約測試：`tools/tests/`。

## 開發原則

- 一般變更直接推 `origin/main`，不開功能分支、不開維護 PR。只有在需要他人審查、或改動風險高到值得先讓 CI 在 PR 上跑一輪時，才退回 **branch → PR → CI → merge**。
- 修 bug 先補可重現失敗測試，再做最小修正。
- 使用繁體中文回覆；使用者文件以繁中為主，公開入口同步維護 `README.en.md`。直接交付可驗證結果，避免冗長背景鋪陳。
- 一般變更提交前跑 `pwsh -NoProfile -File tools\dev_check.ps1` 作為維護 gate，產品變更執行 `pwsh -NoProfile -File scripts\smoke-test.ps1`。
- 提交訊息用 Conventional Commit。Dependabot 或外部 fork 的變更走 PR，讀 diff 並通過 CI 後再合併。
- `REVIEW.md` 是風險快照，不是流水帳。
- 不 force-push `main`，不刪 `upstream` remote。

## 上游處理

1. `git fetch upstream main`
2. `python tools/check_upstream_updates.py --strict`
3. 逐筆判斷是否與繁中 README、Windows gate、發佈閘門或測試衝突。
4. 可同步的提交用 merge；只需要部分修正時 cherry-pick 或最小重做。
5. 跑 `pwsh -NoProfile -File tools\dev_check.ps1`
6. 採用／略過寫進 `docs/DECISIONS.md`，驗證後才推進 `tools/upstream_baseline.json`

Baseline 代表「已審查」，不代表「全部已合併」。

## 依賴新鮮度

`Dependency freshness` 檢查 `tools/check_dependency_freshness.py`，比對宣告與 PyPI/npm/Actions 現行版。
紅燈只有兩種正當出口：
- **維持宣告**：在宣告那一行加 `# freshness-hold: <理由>`。
- **已延後**：在 `.github/dependency-deferrals.json` 加一筆 `{"deferredLatest": "<當時看到的版本>", "reason": "<為什麼這次不升>"}`。

## 驗證

```powershell
pwsh -NoProfile -File tools\dev_check.ps1
pwsh -NoProfile -File scripts\smoke-test.ps1
```

沒有實際跑過 Windows gate 與產品測試，不要宣稱本機開發環境已可用。

## 文件責任

- `README.md` / `README.en.md`：公開產品與 fork 入口。
- `FORK.md`：與上游的關係、差異、同步方式。
- `NOTICE.md`：授權與 attribution。
- `docs/UPSTREAM.md`：upstream remote 與審查清冊。
- `docs/DEVELOPMENT.md`：本機開發與驗收指令。
- `docs/DECISIONS.md`：長期取捨。
- `REVIEW.md`：全庫風險快照。
