# 維護決策

## 2026-10-02：建立 Windows-first 維護型 fork

**決定**：fork `sybxxx/codex-auto-retry`，保留 MIT License 與完整歷史。本線預設分支用 `main`。本線聚焦繁中文件、Windows 開發 gate、Windows 原生驗收，以及逐筆審查的上游追蹤。

**理由**：`codex-auto-retry` 是一套為 Windows Codex 打造的原地自動續跑工具。本 fork 補足 Windows 11 原生開發／驗收骨架、繁體中文維護入口，以及可審計的上游追蹤機制。

**限制**：
- 不把 fork 包裝成原創專案，不移除原作者 sybxxx (TQY Local Tools) 與官方連結。
- 不修改核心 Go 二進位檔架構或破壞官方 IPC 命名管道安全設計（介面字串在地化不在此限，見 2026-10-03 決定）。
- 維護 gate 不預設安裝重型套件，僅需 Python 測試與標準 PowerShell。
- 上游更新必須逐筆審查。

## 2026-10-02：繁中與英文雙語架構，移除簡體中文與非 Windows 程式碼

**決定**：公開說明僅維護繁體中文主檔 `README.md` 與英文鏡像 `README.en.md`，移除簡體中文檔案。同時移除非 Windows 平台的 stub 程式碼（如 `*_nonwindows.go`），專注純 Windows 11 原生架構。

**理由**：維護者首選繁體中文與國際英文雙語維護，且本 fork 定位為純 Windows 原生維護線，排除非 Windows 程式碼與說明可減少維護面雜訊。

## 2026-10-03：產品介面與入口檔名改為繁體中文

**決定**：系統匣、設定視窗、內嵌面板、MCP 工具說明、安裝提示與 `release/windows/` 入口檔名全部改為臺灣繁體中文（`安裝.cmd`、`解除安裝.cmd`、`啟動管理員.cmd`、`安全啟動Codex.vbs`、`README-安裝說明.txt`）；預設後備重試文字由「继续」改為「繼續」。轉換由 `tools/convert_zh_hant.py`（OpenCC `s2twp` 加用語對照表）完成，`test_product_strings_are_traditional_chinese` 防止回歸。

**理由**：維護者要求只留繁體中文與英文。逐字手改無法跟上上游，所以用可重跑的工具；同步上游後重跑即可。

**限制**：
- 既有使用者設定檔裡的後備重試文字不會被改寫，只有新安裝採用新預設。
- 測試裡「UTF-8 被當成 GBK」的亂碼字串必須原樣保留，工具依標記跳過。
- `assets/` 截圖由 `tools/capture_screenshots.ps1` 重拍（2026-10-03）；啟動管理員介面只有英文，沿用上游截圖。

## 2026-10-03：提供單檔安裝程式，不做免安裝版

**決定**：`scripts/build-release.ps1` 除了壓縮檔，另外產生 `Codex-Auto-Retry-<版本>-windows-x64-setup.exe`。它是 `scripts/installer/` 的小程式後面直接接上同一份壓縮檔；執行時把壓縮檔解到暫存資料夾，再呼叫其中的 `deploy.ps1`。

**理由**：本工具必須註冊開機啟動、Codex 外掛與背景服務，沒有「免安裝直接執行」的形態；能省的是解壓縮與找入口檔這一步。安裝邏輯仍只有 `deploy.ps1` 一份，完整性檢查與失敗回復不會出現第二套實作。

**限制**：未做程式碼簽章，SmartScreen 會提示未知發行者；自解壓執行檔也較容易被防毒軟體誤判，所以壓縮檔照常提供。

## 2026-10-02：上游檢查涵蓋 Commit、PR 與 Issue 三面向

**決定**：`check_upstream_updates.py` 以 `--state all` 收集上游 PR 與 Issue，並追蹤 Commit SHA。`gh` 失敗時 fail closed（exit 2）。

**理由**：未合併即關閉的 PR 與待處理的 Issue 同樣可能揭露重要缺陷或需求。排程報告必須確保「未檢查」與「沒有新變更」截然分明。

## 2026-10-02：日常直接推 main

**決定**：日常維護修改在本機跑 `tools\dev_check.ps1` 後直接推 `origin/main`。Dependabot 與外部貢獻仍走 PR，合併前讀 diff。

**理由**：對齊 SanHsien 體系其他維護 fork 的治理規範。
