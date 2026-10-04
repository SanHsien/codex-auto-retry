# 變更紀錄

本專案所有顯著變更均記錄於此檔。

格式基於 [Keep a Changelog](https://keepachangelog.com/zh-TW/1.1.0/)，
並遵循 [語意化版本](https://semver.org/lang/zh-TW/)。
本紀錄僅追蹤本 fork 的維護歷史；上游發佈紀錄請參閱 [`docs/releases/`](docs/releases/)。

[English](CHANGELOG.en.md) | 繁體中文

## [Unreleased]

## [1.0.0] - 2026-10-04

本 fork 改用自己的語意化版本線，從 1.0.0 起算；產品邏輯對應上游 `sybxxx/codex-auto-retry` 0.7.12。先前的 `0.7.12-fork.N` 已全部收進這一版，release 頁只保留 1.0.0。

### Added
- 整套繁體中文／英文雙語：
  - 設定視窗、啟動管理員、內嵌管理面板與系統匣共用一個語言設定（`ui-language.json`，預設繁中），面板右上角與啟動管理員都能切換；新增 MCP 工具 `set_ui_language`。
  - 系統匣提示與通知、面板回覆訊息、佇列名稱、重試上限警告依語言顯示。
  - 安裝程式、執行檔選單、`.cmd` 入口、命令列進度與錯誤訊息（約 160 條）一律中英並列；記憶體警示與關閉 Codex 提示也改為中英並列。
  - MCP 工具標題與說明改為中英並列；安裝說明文字檔加上完整英文版。
- `assets/panel_en.png`：英文版面板截圖。
- 契約測試：命令列與錯誤訊息必須中英並列、含中文的腳本必須有 BOM、面板靜態文字必須有英文。

### Changed
- 版本號：外掛、背景服務、面板改為 `1.0.0`，發佈檔名為 `Codex-Auto-Retry-1.0.0-windows-x64.*`。
- 面板等待策略按鈕改為「翻倍／等差／固定」（英文 Double／Linear／Fixed），兩種語言都能放成一行。
- `tools/convert_zh_hant.py` 只轉換真的含簡體字的行，避免把已是繁體的字（登錄、通過、項目）誤改。

## [0.7.12-fork.5] - 2026-10-03

### Changed
- 執行檔改名為 `Codex-Auto-Retry-<版本>-windows-x64.exe`（拿掉 `-setup`）。
- 啟動管理員改為繁體中文介面（狀態欄位、按鈕、確認對話框），右上角可切換英文，語言偏好與設定視窗共用。
- 安全啟動失敗與找不到安全啟動程式的提示視窗改為繁體中文。
- `assets/startup_manager.png` 重拍為繁體中文介面。
- 直接雙擊執行檔改為顯示繁體中文選單：安裝或更新、開啟啟動管理員、緊急停用、解除安裝、解除安裝並刪除資料（需輸入 Y 確認）、取出整包檔案，並顯示這個安裝檔與電腦上已安裝的版本。原有參數保留，另新增 `-install` 可略過選單直接安裝。

## [0.7.12-fork.4] - 2026-10-03

### Added
- 單檔安裝程式新增 `-startup-manager`，用安裝檔就能開啟啟動管理員（狀態、開機啟動、啟停服務、解除安裝）。
- `tools/capture_screenshots.ps1`：用暫存假資料重拍 `assets/` 截圖。

### Changed
- `assets/settings_zh.png`、`settings_en.png`、`panel.png` 重拍為繁體中文與目前版本的介面。

## [0.7.12-fork.3] - 2026-10-03

### Fixed
- 全新安裝時，只要 Codex 設定裡有任何一個來源已失效的外掛市集（例如資料夾已刪除），`codex plugin list --json` 就會失敗，安裝程式因此以 `configuration_error` 中止。現在改為退回只列出本外掛所屬的市集；解除安裝腳本同樣處理。錯誤訊息補上用 `codex plugin list` 查原因的提示。

## [0.7.12-fork.2] - 2026-10-03

### Added
- 單檔安裝程式 `Codex-Auto-Retry-<版本>-windows-x64-setup.exe`（`scripts/installer/`）：雙擊即安裝，另支援 `-uninstall`、`-remove-data`、`-safe-disable`、`-extract`。
- `tools/convert_zh_hant.py`：把上游簡體產品字串轉成臺灣繁體，同步上游後可重跑。

### Changed
- 系統匣、設定視窗、內嵌面板、MCP 工具說明與安裝提示改為繁體中文；預設後備重試文字由「继续」改為「繼續」（既有設定不受影響）。
- `release/windows/` 入口改名為 `安裝.cmd`、`解除安裝.cmd`、`啟動管理員.cmd`、`安全啟動Codex.vbs`、`README-安裝說明.txt`。

### Fixed
- `scripts/build-release.ps1` 打包清單仍指向已改名的 `README_zh.md`，導致無法產生發佈檔；改為 `README.md` 與 `README.en.md`，並加入契約測試。
- 移除 `*_nonwindows.go` 後重建 `scripts/bin/` 兩個執行檔與 `scripts/build-info.json`，來源雜湊重新對齊。
- 在原始碼的 `release\windows\` 直接執行 `安装.cmd` 時，`deploy.ps1` 改為明確說明缺少 `release-manifest.json` 以及正確安裝方式；README 補上從原始碼安裝的步驟。
- `docs/project-map.md` 移除已刪除的非 Windows 檔案說明；`.gitignore` 移除 `.DS_Store` 並忽略 `scripts/source/*.exe`。

## [0.7.12-fork.1] - 2026-10-02

### Added
- 初始化 SanHsien 專用之 Windows-first 維護型 fork。
- 建立繁體中文入口主檔 [`README.md`](README.md)，英文鏡像設為 [`README.en.md`](README.en.md)。
- 移除非 Windows 平台之 Go stub 程式碼（`*_nonwindows.go`）與相關檔案，專注純 Windows 11 原生架構。
- 移除簡體中文 README 檔案。
- 建立 AI 維護指引單一真相源 [`AGENTS.md`](AGENTS.md)。
- 建立 Fork 關係說明 [`FORK.md`](FORK.md) 與授權宣告 [`NOTICE.md`](NOTICE.md)。
- 加入 `.cursor/rules/no-upstream-pr.mdc` 機器層防護，防止誤向上游開 PR。
- 建立 Windows 原生本機開發與維護門禁腳本 `tools/dev_check.ps1`。
- 建立上游水位檢查工具 `tools/check_upstream_updates.py` 與基準點 `tools/upstream_baseline.json`（鎖定 baseline `867da68`）。
- 建立依賴新鮮度追蹤器 `tools/check_dependency_freshness.py` 與相對連結檢查器 `tools/check_links.py`。
- 建立文件與決策紀錄：[`docs/UPSTREAM.md`](docs/UPSTREAM.md)、[`docs/DECISIONS.md`](docs/DECISIONS.md) 與 [`docs/DEVELOPMENT.md`](docs/DEVELOPMENT.md)。
- 建立全庫風險快照 [`REVIEW.md`](REVIEW.md)。
