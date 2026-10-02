# 維護決策

## 2026-10-02：建立 Windows-first 維護型 fork

**決定**：fork `sybxxx/codex-auto-retry`，保留 MIT License 與完整歷史。本線預設分支用 `main`。本線聚焦繁中文件、Windows 開發 gate、Windows 原生驗收，以及逐筆審查的上游追蹤。

**理由**：`codex-auto-retry` 是一套為 Windows Codex 打造的原地自動續跑工具。本 fork 補足 Windows 11 原生開發／驗收骨架、繁體中文維護入口，以及可審計的上游追蹤機制。

**限制**：
- 不把 fork 包裝成原創專案，不移除原作者 sybxxx (TQY Local Tools) 與官方連結。
- 不修改核心 Go 二進位檔架構或破壞官方 IPC 命名管道安全設計。
- 維護 gate 不預設安裝重型套件，僅需 Python 測試與標準 PowerShell。
- 上游更新必須逐筆審查。

## 2026-10-02：繁體中文主檔與多語鏡像架構

**決定**：`README.md` 為繁體中文主檔；原簡體中文說明搬移至 `README.zh-CN.md` 作為鏡像對照；提供 `README.en.md` 英文鏡像。

**理由**：維護者習慣以繁體中文作為首選閱讀入口，同時保留原作者簡中說明以利後續與上游對齊，並維護英文版本作為國際通用說明。

## 2026-10-02：上游檢查涵蓋 Commit、PR 與 Issue 三面向

**決定**：`check_upstream_updates.py` 以 `--state all` 收集上游 PR 與 Issue，並追蹤 Commit SHA。`gh` 失敗時 fail closed（exit 2）。

**理由**：未合併即關閉的 PR 與待處理的 Issue 同樣可能揭露重要缺陷或需求。排程報告必須確保「未檢查」與「沒有新變更」截然分明。

## 2026-10-02：日常直接推 main

**決定**：日常維護修改在本機跑 `tools\dev_check.ps1` 後直接推 `origin/main`。Dependabot 與外部貢獻仍走 PR，合併前讀 diff。

**理由**：對齊 SanHsien 體系其他維護 fork 的治理規範。
