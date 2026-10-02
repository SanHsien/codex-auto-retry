# Repository review（Windows-only）

- Review date: 2026-10-02
- Review baseline: `867da682d1863c4d4eb142aab3078aca4df8e0f3`
- Remediation: 同日 fork-local overlay（不回貢）
- Upstream reviewed through: `867da682d1863c4d4eb142aab3078aca4df8e0f3`
- Primary environment: Windows 11、PowerShell、Go 1.23+、Node.js v22+、Python 3.10+（本機 gate）
- Status: 維護骨架與產品相依環境全面可用。已完成建立 Windows 原生門禁與驗收。

## 結論

這個 fork 適合作為 Windows 本機、給 Agent 維護的 Codex 自動續跑與可靠性守護線。產品行為跟隨 `sybxxx/codex-auto-retry` `867da68`，再加上本線維護骨架：繁體中文維護文件、Windows 原生一鍵門禁（`tools/dev_check.ps1`）、每週上游水位追蹤（commit、PR、issue）以及每月依賴新鮮度檢查。

本 repo 的維護依賴（`pytest`、`ruff`）與 Go / Node / PowerShell 執行環境皆已完整梳理並通過 Windows 原生環境驗證。在 Windows 環境下執行腳本時，門禁與工具腳本全面注入 `$env:PYTHONUTF8 = "1"`，避免預設 ANSI/CP950 編碼解碼 UTF-8 文件失敗。

## 本輪實證

### 審查當下（`867da68`）

```text
git rev-parse HEAD
→ 867da682d1863c4d4eb142aab3078aca4df8e0f3

gh repo set-default --view
→ SanHsien/codex-auto-retry
```

實查結果：
- 上游 repository 為 `sybxxx/codex-auto-retry`，採 MIT License。
- 上游 PR 水位為 `#0`，Issue 水位為 `#0`。
- 上游已有 Windows CI 工作流程（`ci.yml`），且包含多項 PowerShell smoke tests。
- 維護工具無 `os.system`／`shell=True`／`eval(`／`exec(`。

## 已修 findings

| ID | 嚴重度 | 做了什麼 |
|---|---|---|
| R-01 | P2 | `.gitignore` 加入 `.env`、`.venv`、`upstream-review-report.md`、`dependency-freshness-report.md`、`.ruff_cache/`、`.pytest_cache/` |
| R-02 | P2 | 建立獨立維護測試目錄 `tools/tests/` 與獨立 `tools/pytest.ini`，隔離維護測試 |
| R-03 | P2 | 建立 `FORK.md`、`NOTICE.md`、`AGENTS.md`，寫明對外邊界與安全性 |
| R-04 | P2 | 建立 `.cursor/rules/no-upstream-pr.mdc`，防止誤向上游開 PR |
| R-05 | P3 | 建立雙語說明與鏡像，主檔 `README.md`（繁中）、`README.en.md`（英文）與 `README.zh-CN.md`（簡中）互聯 |

## 接受、不改契約

- 上游既有 Go 核心二進位檔與命名管道 IPC 通訊架構（`\\.\pipe\codex-ipc`）完整保留。
- 上游既有 PowerShell 部署與管理腳本（`release/windows/` 與 `scripts/`）完整保留。

## 尚未宣稱範圍

- 非 Windows 作業系統（macOS / Linux）不在本 fork 支援與驗收範圍內。
- 永久失效之登入憑證不在自動續跑範圍內，需由使用者於 Codex Desktop 重新登入。
