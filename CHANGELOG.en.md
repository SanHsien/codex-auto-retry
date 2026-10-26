# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
This log tracks maintenance changes in this fork only; upstream releases are documented in [`docs/releases/`](docs/releases/).

English | [繁體中文](CHANGELOG.md)

## [Unreleased]

### Fixed
- **Quota exhaustion no longer ends retrying for good**: when the error says "try again at ...", the retry is scheduled after that reset and the automatic recovery window starts at the reset. For `rate_limit` failures, a temporarily unreachable controller (including a missing owner for a subagent thread) now backs off and retries instead of stopping after 3 failures. `usage limit` messages and plan/billing quota errors are classified as `rate_limit`.
- **Root cause of `controller_unavailable`**: the PowerShell desktop-transport check was killed after 8 seconds on a busy machine, while the lifecycle probe that contains it only had 5. The check now allows 20 seconds, the probe 45, and a timeout is reported as `controller_timeout`.

### Security
- The panel bundle no longer contains Zod's dynamic `Function` (CodeQL `js/bad-code-sanitization`); the build fails if the output contains `new Function(`.

## [1.2.0] - 2026-10-17

### Added
- **Graphical Installer & Control Center (`release/windows/installer-gui.ps1`)**: Double-clicking `Codex-Auto-Retry-<version>-windows-x64.exe` now opens a modern WinForms window instead of the `1~6` console menu, offering one-click install/update, optional Desktop shortcut and shared-backend checkboxes, live log output, and direct buttons for the Startup Manager, Retry Settings, config/log folder, `daemon.log`, Safe Disable, Uninstall (keep settings or remove all data), and package extraction. Added `-cli` (classic text menu) and `-create-desktop-shortcut` CLI flags.
- **Windows Start Menu & "Installed Apps" Integration (`scripts/shell-integration.ps1`)**: Installing now copies self-contained management scripts and icons into `%LOCALAPPDATA%\CodexAutoRetry`, creates a "Codex Auto Retry" Start Menu shortcut group (Startup Manager, Settings, Open Config & Log Folder, Safe Disable, Uninstall) plus an optional Desktop shortcut, and registers a per-user `HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\CodexAutoRetry` entry, cleaning them all up on uninstall.
- **Config & Log Path Visibility**: The Settings window (`scripts/source/ui/settings.ps1`), Startup Manager (`scripts/startup-manager.ps1`), Installer GUI, and System Tray right-click menu (`scripts/source/tray_windows.go`) now display the data directory (`%LOCALAPPDATA%\CodexAutoRetry`), `config.json`, and `logs\daemon.log` paths directly, with one-click buttons/menu items to open the folder, view `daemon.log`, and open the Startup Manager.
- **Custom App Icon & 5-State ARGB System Tray Icons (`tools/generate_icons.py`, `scripts/source/tray_icons_windows.go`)**: Embedded a multi-resolution (16/24/32/48/256 px) `.ico` via PE COFF `.syso` resources into all Windows `.exe` binaries and WinForms windows, and replaced generic system tray icons with anti-aliased 32-bit ARGB icons reflecting each daemon state (`running` emerald, `active` blue, `waiting` amber, `paused` slate, `stopped` coral red).

## [1.1.1] - 2026-10-08

### Fixed
- Running the installer exe or a `.cmd` from a PowerShell 7 terminal failed at "Verifying release files" because `Get-FileHash` was not found. The inherited PowerShell 7 module path (`PSModulePath`) made Windows PowerShell 5.1 load incompatible modules; the variable is now cleared before launch. Double-clicking from File Explorer was not affected.

## [1.1.0] - 2026-10-05

### Added
- The settings window queue shows Codex task titles (read from Codex's `session_index.jsonl`; only the title is read, never conversation content, and it is neither stored nor logged; hover for the full ID). Titles stay in the local settings window and are not added to the panel data returned to Codex.
- Refresh and Find Interrupted buttons; the panel gains a Find interrupted button and the MCP tool `rescan_interrupted_tasks`. The rescan lists tasks from the last 24 hours whose last turn ended with a retryable failure and was never continued, as Interrupted; they wait for Restart and are never sent automatically.

### Changed
- Cancelling a pending retry no longer removes it: it stays in the queue as Cancelled for 24 hours and can be restarted, and it clears when the task is continued manually in Codex.

### Fixed
- Control commands accepted an empty task ID.

## [1.0.0] - 2026-10-04

This fork now has its own semantic version line starting at 1.0.0; product logic matches upstream `sybxxx/codex-auto-retry` 0.7.12. All earlier `0.7.12-fork.N` builds are folded into this release, and only 1.0.0 remains on the release page.

### Added
- Full Traditional Chinese / English support:
  - The settings window, startup manager, embedded panel, and tray share one language preference (`ui-language.json`, Traditional Chinese by default), switchable from the panel header and the startup manager; new MCP tool `set_ui_language`.
  - Tray tooltips and notifications, panel notices, queue labels, and the retry-limit warning follow the language.
  - The installer, the executable's menu, `.cmd` launchers, console progress, and error messages (about 160) always show Chinese and English side by side; so do the memory alert and the close-Codex prompt.
  - MCP tool titles and descriptions are bilingual; the installation guide text file gains a full English section.
- `assets/panel_en.png`: English panel screenshot.
- Contract tests: console and error messages must be bilingual, scripts with Chinese text must carry a BOM, and panel static text must have English.

### Changed
- Version: plugin, watchdog, and panel are `1.0.0`; release files are `Codex-Auto-Retry-1.0.0-windows-x64.*`.
- Panel wait-strategy buttons read 翻倍／等差／固定 (Double / Linear / Fixed) so both languages fit on one line.
- `tools/convert_zh_hant.py` only converts lines that really contain Simplified characters, so valid Traditional words (登錄, 通過, 項目) are no longer rewritten.

## [0.7.12-fork.5] - 2026-10-03

### Changed
- The executable is renamed to `Codex-Auto-Retry-<version>-windows-x64.exe` (no `-setup`).
- The startup manager now has a Traditional Chinese UI (status labels, buttons, confirmation dialogs) with an English toggle; the language choice is shared with the settings window.
- The safe-launch failure and missing-launcher dialogs are now in Traditional Chinese.
- `assets/startup_manager.png` is retaken with the Traditional Chinese UI.
- Double-clicking it now shows a Traditional Chinese menu: install/update, startup manager, safe-disable, uninstall, uninstall and remove data (requires typing Y), and extract, together with the packaged and installed versions. Existing flags still work; the new `-install` flag installs without the menu.

## [0.7.12-fork.4] - 2026-10-03

### Added
- The single-file installer gains `-startup-manager`, so the startup manager (status, sign-in startup, service start/stop, uninstall) is reachable without the ZIP.
- `tools/capture_screenshots.ps1` retakes the `assets/` screenshots from temporary sample data.

### Changed
- `assets/settings_zh.png`, `settings_en.png`, and `panel.png` now show the current Traditional Chinese UI.

## [0.7.12-fork.3] - 2026-10-03

### Fixed
- On a fresh install, any configured Codex marketplace with a missing or invalid source (for example a deleted folder) made `codex plugin list --json` fail, so the installer stopped with `configuration_error`. It now falls back to listing only this plugin's own marketplace; the uninstaller does the same. The error message now points to `codex plugin list` for the cause.

## [0.7.12-fork.2] - 2026-10-03

### Added
- Single-file installer `Codex-Auto-Retry-<version>-windows-x64-setup.exe` (`scripts/installer/`): double-click to install; also supports `-uninstall`, `-remove-data`, `-safe-disable`, and `-extract`.
- `tools/convert_zh_hant.py` converts upstream Simplified Chinese product strings to Traditional Chinese (Taiwan) and can be re-run after an upstream sync.

### Changed
- Tray, settings window, embedded panel, MCP tool descriptions, and installer prompts are now Traditional Chinese; the default fallback retry prompt changes from "继续" to "繼續" (existing configs are untouched).
- `release/windows/` launchers are renamed to `安裝.cmd`, `解除安裝.cmd`, `啟動管理員.cmd`, `安全啟動Codex.vbs`, and `README-安裝說明.txt`.

### Fixed
- `scripts/build-release.ps1` still listed the renamed `README_zh.md` in the payload, so no release archive could be built; it now ships `README.md` and `README.en.md`, guarded by a contract test.
- Rebuild both `scripts/bin/` executables and `scripts/build-info.json` after removing `*_nonwindows.go`, so the source hash matches again.
- Running `安装.cmd` from the source-tree `release\windows\` template now reports the missing `release-manifest.json` and how to install correctly; the READMEs document installing from source.
- Drop deleted non-Windows files from `docs/project-map.md`; remove `.DS_Store` from `.gitignore` and ignore `scripts/source/*.exe`.

## [0.7.12-fork.1] - 2026-10-02

### Added
- Initialize SanHsien's Windows-first maintenance fork.
- Establish Traditional Chinese entry [`README.md`](README.md), and provide English mirror [`README.en.md`](README.en.md).
- Remove non-Windows Go stubs (`*_nonwindows.go`) and files, focusing on Windows 11 native runtime.
- Remove Simplified Chinese README file.
- Create AI maintenance guide as single source of truth [`AGENTS.md`](AGENTS.md).
- Create fork documentation [`FORK.md`](FORK.md) and attribution notice [`NOTICE.md`](NOTICE.md).
- Add `.cursor/rules/no-upstream-pr.mdc` guard rule to prevent accidental pull requests to upstream.
- Create Windows native development gate script `tools/dev_check.ps1`.
- Add upstream tracker `tools/check_upstream_updates.py` with baseline ledger `tools/upstream_baseline.json` pinned at `867da68`.
- Add dependency freshness checker `tools/check_dependency_freshness.py` and relative link validator `tools/check_links.py`.
- Add documentation: [`docs/UPSTREAM.md`](docs/UPSTREAM.md), [`docs/DECISIONS.md`](docs/DECISIONS.md), and [`docs/DEVELOPMENT.md`](docs/DEVELOPMENT.md).
- Add full repository risk review snapshot [`REVIEW.md`](REVIEW.md).
