# 變更紀錄

本專案所有顯著變更均記錄於此檔。

格式基於 [Keep a Changelog](https://keepachangelog.com/zh-TW/1.1.0/)，
並遵循 [語意化版本](https://semver.org/lang/zh-TW/)。
本紀錄僅追蹤本 fork 的維護歷史；上游發佈紀錄請參閱 [`docs/releases/`](docs/releases/)。

[English](CHANGELOG.en.md) | 繁體中文

## [Unreleased]

## [0.7.12-fork.1] - 2026-10-02

### Added
- 初始化 SanHsien 專用之 Windows-first 維護型 fork。
- 建立繁體中文入口主檔 [`README.md`](README.md)，原簡中說明保留為 [`README.zh-CN.md`](README.zh-CN.md)，英文鏡像設為 [`README.en.md`](README.en.md)。
- 建立 AI 維護指引單一真相源 [`AGENTS.md`](AGENTS.md)。
- 建立 Fork 關係說明 [`FORK.md`](FORK.md) 與授權宣告 [`NOTICE.md`](NOTICE.md)。
- 加入 `.cursor/rules/no-upstream-pr.mdc` 機器層防護，防止誤向上游開 PR。
- 建立 Windows 原生本機開發與維護門禁腳本 `tools/dev_check.ps1`。
- 建立上游水位檢查工具 `tools/check_upstream_updates.py` 與基準點 `tools/upstream_baseline.json`（鎖定 baseline `867da68`）。
- 建立依賴新鮮度追蹤器 `tools/check_dependency_freshness.py` 與相對連結檢查器 `tools/check_links.py`。
- 建立文件與決策紀錄：[`docs/UPSTREAM.md`](docs/UPSTREAM.md)、[`docs/DECISIONS.md`](docs/DECISIONS.md) 與 [`docs/DEVELOPMENT.md`](docs/DEVELOPMENT.md)。
- 建立全庫風險快照 [`REVIEW.md`](REVIEW.md)。
