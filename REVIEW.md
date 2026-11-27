# Repository review（Windows-only）

- Review date: 2026-10-03
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
| R-05 | P3 | 建立雙語說明與鏡像，主檔 `README.md`（繁中）與 `README.en.md`（英文）互聯，移除簡體中文與非 Windows 程式碼 |
| R-06 | P1 | `scripts/build-release.ps1` 打包清單仍列 `README_zh.md`（fork 改名後已不存在），發佈檔打不出來、CI 最後一步必紅；改列 `README.md`、`README.en.md`，加 `test_release_payload_entries_exist`（`51285a1`，2026-10-03） |
| R-07 | P1 | 刪除 `*_nonwindows.go` 後未重建，`scripts/build-info.json` 來源雜湊與內附執行檔不符，`Assert-CodexBuildProvenance` 會擋打包；已用 `scripts/build.ps1` 重建（`51285a1`，2026-10-03） |
| R-08 | P2 | 在原始碼 `release\windows\` 直接跑 `安装.cmd` 只回報「not a valid release」，看不出原因；`deploy.ps1` 改為指出缺 `release-manifest.json` 與正確做法，README 補從原始碼安裝步驟（`51285a1`，2026-10-03） |
| R-09 | P3 | `docs/project-map.md` 仍列已刪的非 Windows 檔案、`.gitignore` 留有 `.DS_Store`；已清除並加 `test_fork_is_windows_only`（`51285a1`，2026-10-03） |
| R-10 | P2 | 產品介面、安裝提示與 `release/windows/` 入口檔名仍是上游簡體中文；以 `tools/convert_zh_hant.py` 轉為繁體並同步改打包／驗證腳本的檔名常數，加 `test_product_strings_are_traditional_chinese`（`17c944d`，2026-10-03） |
| R-11 | P3 | 只能下載壓縮檔再找入口檔安裝；新增單檔安裝程式 `scripts/installer/`，`release-test.ps1` 驗證其內容與壓縮檔一致（`17c944d`，2026-10-03） |
| R-12 | P1 | 全新安裝的前置檢查列出全部市集，Codex 設定裡任何一個失效的無關市集都會讓安裝以 `configuration_error` 中止（維護者實機遇到）；`deploy.ps1`、`uninstall-release.ps1` 改為退回只列本外掛的市集，`installer-cli-smoke-test.ps1` 加兩個情境（`b0d225f`，2026-10-03） |
| R-13 | P3 | 用單檔安裝程式安裝的人無法開啟啟動管理員（入口只在壓縮檔裡）；新增 `setup.exe -startup-manager`；`assets/` 截圖以 `tools/capture_screenshots.ps1` 重拍為繁體（`177de48`，2026-10-03） |
| R-14 | P3 | 執行檔只能靠參數切換功能、檔名帶 `-setup` 易誤會成只能安裝；改名並在雙擊時顯示操作選單，刪除資料需輸入 Y 確認（`0ad4be8`，2026-10-03） |
| R-15 | P2 | 啟動管理員與安全啟動的錯誤視窗只有英文；啟動管理員加繁中／英文切換（共用 `ui-language.json`），兩個錯誤視窗改繁中（`0ad4be8`，2026-10-03） |
| R-16 | P2 | 英文使用者只看得到中文介面與錯誤訊息，版本號用 `-fork.N` 尾碼；改為整套中英雙語（可切換或並列）、版本號改為 1.0.0（`72e7641`、`0117a72`，2026-10-04） |
| R-17 | P2 | 設定視窗只顯示任務 ID；取消等待中的重試會直接刪除且無法再偵測到；改為顯示標題、取消後保留 24 小時可重新開始、加入重新偵測中斷任務（`184275d`、`bee25b4`、`20084ec`，2026-10-05） |
| R-18 | P2 | 從 PowerShell 7 終端機執行安裝執行檔或 `.cmd`，Windows PowerShell 5.1 繼承 7 的 `PSModulePath` 而找不到 `Get-FileHash`，安裝在驗證發佈檔時失敗；啟動前清掉該變數。另修共用後端冒煙測試清理暫存資料夾時偶發 EPERM（真機實測發現，2026-10-08） |
| R-19 | P2 | Go 測試直接讀寫真實 `HKCU\Environment`；`TestOwnedSharedEnvironmentRestoresMissingEndpoint` 重新賦值清理閉包捕捉的 `present`，清理時寫入空值而非刪除，每跑一次留一個 `CODEX_AUTO_RETRY_TEST_<nanos>`（維護者電腦累積 33 個）。登錄路徑與環境變更廣播改為可替換，`TestMain` 改用專屬測試子鍵並在結束後刪除，真實 `HKCU\Environment` 多出任何 `CODEX_AUTO_RETRY_*` 就讓套件失敗；閉包問題一併修正（`ae5d007`，2026-11-15） |
| R-20 | P2 | 心跳逾時寫死成 `max(4×輪詢, 15 秒)`，掃描比輪詢慢時（2026-11-21 真實 host 實測間隔 25～50 秒），面板、MCP 與啟動管理員在 worker 活著時回 `running=false`、`heartbeat_stale=true`，並藏起等待中與執行中的任務。worker 改寫出實測心跳間隔 `scan_interval_ms`，逾時改為 4 個實測週期（至少 15 秒，`management.go` 的 `heartbeatStaleAfter`、`startup-manager.ps1` 的 `Get-HeartbeatMaxAge`）。順帶查出並修掉 `last_scan_at` 倒退：`daemon.run` 用 `time.Ticker` 的排定時間當「現在」，前一輪超時後那個值已過時；改用 `time.Now()`。新版尚未安裝到維護者電腦，見 `ROADMAP.md`（`bf059f6`，2026-11-21） |
| R-21 | P2 | `stoppedRetryIsVisible` 的「故障太舊」判斷從 `FailedAt` 起算，等額度超過 1 小時才停止的任務一停止就從面板消失；等額度的任務改從 `QuotaResetAt` 起算，停止後的顯示期間不變（`bf059f6`，2026-11-21） |
| R-22 | P2 | Codex Desktop `26.1117` 改由 NodeService 子行程（`ChatGPT.exe --type=utility --utility-sub-type=node.mojom.NodeService`）啟動 `codex.exe app-server`，`desktopTransportScript` 只認主行程的直接子行程，回 `unknown`，`preflight` 因此不檢查官方 IPC，狀態把 Desktop 傳輸說成無法證明。改為也接受這個子行程，條件是父行程為已驗證的主行程、執行檔路徑相同；它底下的 app-server 才算 Desktop 的，它連到共用埠也算 Desktop 的連線。測試用這台機器實抓的 `26.1117.4957.0` 行程樹（先紅後綠）與 10 個必須維持 `unknown` 的情形。修好後這台機器的判斷由 `unknown` 變成 `legacy_stdio`，但 `recovery_mode` 仍是 `none`：新版 Desktop 沒有官方 IPC 路由，見「未解風險」。已重建 `scripts/bin`（`ebaf23b`，2026-11-22）。**已安裝 2026-11-23**（連同 R-23，兩個 exe `BEF7DEB6…31B7`，共用後端模式），在 `26.1117.4957.0` 上實測連到共用埠的正是這個 NodeService host，`controller_state=ready`、`recovery_mode=shared_websocket`，見 `docs/DECISIONS.md`「2026-11-23」 |
| R-23 | P2 | 傳輸判斷（原 `desktopTransportScript`，PowerShell 的 `Get-CimInstance Win32_Process`＋`Get-NetTCPConnection`）在維護者電腦上一次 5～15 秒，上限 20 秒（`desktopTransportCheckTimeout`），正式流程 4 次逾時 3 次，回報 `controller_timeout`。量到時間幾乎都在 PowerShell 啟動（0.7～1.0 秒）、WMI（不篩選 1.3～1.6 秒、篩選後仍 0.9 秒）與載入 NetTCPIP 模組（`Get-NetTCPConnection` 第一次 3.6 秒）。改用 Win32 直接讀：`CreateToolhelp32Snapshot`，只對 `ChatGPT.exe`／`Codex.exe` 讀執行檔路徑與命令列（`ProcessCommandLineInformation`），有主行程才讀 `GetExtendedTcpTable`；R-22 的身分條件照搬進 `classifyDesktopTransport`，原本 25 個 fixture 情形期望值不變。這台機器傳輸判斷 p50／p95 由 5.57／8.18 秒降到 27／32 毫秒，`Readiness` 由 6.34／13.53 秒降到 34／37 毫秒，各 10 次 0 逾時；`TestDesktopTransportLiveProbeStaysFarBelowTimeout` 要求實跑每次 1 秒內。覆核補修 H1：查詢前先對用到的 Win32 進入點 `Find()`，找不到 DLL 或函式時回錯誤（傳輸當成 `unknown`），不再在 goroutine 裡 panic 讓守護程式結束；goroutine 也會 `recover`（`c83793e`，2026-11-22）。已重建 `scripts/bin`（`729e008`，2026-11-22）。**已安裝 2026-11-23**，安裝後連看 5 分鐘 `controller_state` 都是 `ready`，沒有再出現 `controller_timeout` |

## 未解風險

- Codex Desktop `26.1117` 以後沒有官方 IPC 恢復通道（2026-11-22 查實），唯一的恢復路徑是共用後端。2026-11-23 維護者電腦已改用共用後端模式並實測接續成立（見 `docs/DECISIONS.md`「2026-11-23」）；但只要 Desktop 不是由安全啟動器（`scripts\launch-codex.ps1`）開的，就會回到官方 stdio、`recovery_mode=none`，只能監看。Desktop 自己開的任務接續、goal-limit 停止仍沒有在真實 host 驗到，見 `ROADMAP.md`。
- Actions 已啟用 push 觸發：2026-10-30 唯讀查詢確認 `855973b2f350377ea598cceea9369fc8bc565b14` 的 [CI run 40745326049](https://github.com/SanHsien/codex-auto-retry/actions/runs/40745326049) 為 `event=push`、`conclusion=success`（建立於 2026-10-28 18:27:41 UTC，即臺北時間 2026-10-29）。舊的 fork workflow 停用敘述已過期；不需再以手動觸發作為固定補救。`go test -race` 的 gcc 需求不變，此次本機維護驗收不涵蓋競態測試。
- 發佈檔未做程式碼簽章；單檔安裝程式是自解壓執行檔，SmartScreen 與防毒軟體可能示警。
- 繁體化後每次同步上游都要重跑轉換工具並人工對照 PowerShell 測試裡的字元碼常數。
- `scripts/status-smoke-test.ps1` 依賴本機實際安裝狀態，在沒安裝的電腦上失敗，改動前後相同。

## 啟動 fail-open smoke 複驗（2026-10-30）

- 基準：detached HEAD `855973b2f350377ea598cceea9369fc8bc565b14`；Windows 11 專業版 `10.0.26300`（build 26300）、PowerShell 7.6.9、Windows PowerShell 5.1.26100.9711；使用 repo 內附執行檔，不涉及安裝版更新。
- 隔離：子行程使用唯一暫存資料目錄、空 session roots、關閉預設 home 掃描；修正後另指定隔離 `CODEX_HOME`。`TEMP`／`TMP` 指向 repo 的 `dist/task-evidence/temp`；正式 User endpoint 僅讀取、比對，不寫入。
- 根因：舊測試只占用連接埠，沒有失效的共享後端 ownership 紀錄；當官方 IPC 可用時，controller preparation 可直接成功，所以連接埠衝突不保證觸發 fail-open。這是 smoke 前提過時，並非已證實的產品 fail-open 缺陷。
- 最小修正：建立已確認不在執行的 PID 之隔離 owned state，讓 startup gate 在 controller preparation 前執行 fail-open；同時核對設定與 heartbeat 都已停用共享模式。原因改查持久的 `shared-availability.json`，避免第一個 tick 更新 `controller_state` 後誤判；不修改產品 Go 程式或二進位檔。
- 先紅後綠使用同一入口（完整外層指令如下）；初次原版退出碼 1：`A failed shared backend was left enabled during startup.`；補診斷後仍為 1：`controller_state=ready; last_error=`。修正後退出碼 0：`Status=passed`、`StartupHeartbeat=True`、`SharedModeDisabled=True`、`FailOpenReasonPublished=True`、`FailOpenReason=codex_background_channel_unavailable`、`ProductionEndpointUntouched=True`。原始輸出分別保存在本機 `dist/task-evidence/startup-fail-open.log`、`startup-red.log`、`startup-green.log`（忽略的本機證據，不納入版本控制）。

```powershell
$env:TEMP = Join-Path (Get-Location) 'dist/task-evidence/temp'
$env:TMP = $env:TEMP
powershell.exe -NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -File scripts/startup-fail-open-smoke-test.ps1
```

這次複驗補上個別啟動情境的本機證據；CI 仍未直接執行這支 smoke，不能拿整體 CI 成功取代這項驗收。

## 接受、不改契約

- 上游既有 Go 核心二進位檔與命名管道 IPC 通訊架構（`\\.\pipe\codex-ipc`）完整保留。
- 上游既有 PowerShell 部署與管理腳本（`release/windows/` 與 `scripts/`）完整保留。

## 尚未宣稱範圍

- 非 Windows 作業系統（macOS / Linux）不在本 fork 支援與驗收範圍內。
- 永久失效之登入憑證不在自動續跑範圍內，需由使用者於 Codex Desktop 重新登入。
## 2026-11-27：安全啟動冷開準備時限

安全啟動冷開後端時，準備時限只有 6 秒，短於後端自身最長 12 秒的就緒等待。`de6cc20`（2026-11-27）將 worker 時限調為 18 秒、啟動器請求有效期調為 20 秒，仍保留請求到期及官方連線備援。回歸測試在修正前失敗，修正後通過。真機紀錄只有 `backend_prepare_failed`，仍須安裝新版後驗收桌面接續。
