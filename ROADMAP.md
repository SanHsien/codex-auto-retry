# 路線圖

> 本 repo 後續工作的單一真相源。做完一項，就在同一個 commit 刪掉那一列；新工作只加在這裡。
> 現況段的 GitHub 數字是 2026-11-08 依 `main` `c19b1e6` 查的快照，已安裝版本與 `scripts/build-info.json` 是 2026-11-22 實查；開工前先重查。

## 現況
這是 Windows Codex Desktop 自動接續守護工具的維護 fork（[AGENTS.md:7](AGENTS.md)）。2026-11-08 `gh` 查詢：`c19b1e6` 的 CI、CodeQL 為 success，code scanning 開啟告警 0 筆，最近 release 為 v1.2.0；issues 停用，未查到 issue。2026-11-14 以 `scripts/install.ps1` 安裝 `966329c` 重建的 `scripts/bin`（面板技術細節保留展開），已安裝的 `%LOCALAPPDATA%\CodexAutoRetry\codex-auto-retry.exe` 與 `codex-auto-retry-mcp.exe` 的 SHA-256 都是 `D3FD195FA22C92ED153D73FDA64B28A9635F6CAE732801CCE7E97A94AB60DE2A`，等於當時的 `scripts/build-info.json`。2026-11-21 修好 R-20、R-21 後重建，2026-11-22 完全關閉 Codex Desktop 後以 `scripts/install.ps1` 裝上：兩個已安裝 exe 的 SHA-256 都是 `1E849611EF072EE00F3854A73B9BBD6DD90703342CEB99D064966F9A337E95EA`，等於 `scripts/build-info.json`，`status.json` 的 `build_source_hash` 等於 build-info 的 `source_hash`（`82C69F56…58F3`）。同日用 `scripts/launch-codex.ps1` 重開 Codex Desktop（`OpenAI.Codex_26.1117.4957.0`）後，`recovery_mode` 仍是 `none`，見下表第一列。額度等待與面板修正仍在 Unreleased；深掃報告（2026-10-27）指出當時已安裝版尚未更新、真實 MCP host 未驗，已於 2026-11-08 核對，見上（[CHANGELOG.md:11](CHANGELOG.md)）。

## 近期：可以直接動手

| 項目 | 為什麼 | 驗收方式 | 建議執行者 | 證據 |
| --- | --- | --- | --- | --- |
| 查清 Codex Desktop `26.1117.4957.0` 下恢復通道為什麼不可用，並決定怎麼接 | 2026-11-22 用 `scripts/launch-codex.ps1` 重開後 `recovery_mode` 仍是 `none`、`controller_state=shared_app_server_disabled`，所有重試照樣停在派送前；「不是由 `launch-codex.ps1` 開的」這個推測不成立。實查到兩件事：`\\.\pipe\codex-ipc` 不存在（`dialCodexIPC` 開不到）；`codex.exe app-server` 的父行程是 `ChatGPT.exe --type=utility --utility-sub-type=node.mojom.NodeService`，不是主行程，而 `desktopTransportScript` 只認主行程的直接子行程，所以回 `unknown`，連官方 IPC 都不會去試。2026-11-17 的紀錄還有 `retry action started`，當時通道是通的；中間哪一版 Desktop 改掉的沒有查到 | 先唯讀確認目前這版 Desktop 有沒有別的 owner 路由（管道名稱、是否要某個條件才建立）。有：更新 `codex_ipc_windows.go` 與 `desktopTransportScript`（認得 NodeService 底下的 app-server），加上先紅後綠的測試，重建、安裝，真實 host 上 `controller_state=official_ipc_ready`。沒有：文件與面板改寫成「這版 Desktop 只能監看」，並評估共用後端模式在 app-server 改由 NodeService 啟動後還能不能被 `desktopTransportScript` 認出來（它只看主行程與 NetworkService 的連線） | Claude 主 session（先讀 `docs/shared-backend-safety.md` 的 Capability Profiles；不要為了驗收直接對維護者的 Desktop 開共用後端） | [docs/DECISIONS.md](docs/DECISIONS.md)「2026-11-22」；[scripts/source/desktop_transport_windows.go:56](scripts/source/desktop_transport_windows.go)；[scripts/source/codex_ipc_windows.go:20](scripts/source/codex_ipc_windows.go) |
| 在真實 Codex Desktop／MCP host 驗證實際接續與 goal-limit 停止 | 其餘面板與 MCP 操作已在 2026-11-14、11-21、11-22 三次真實 host 驗收完成，細節見 `docs/DECISIONS.md`。還沒驗兩件事。第一，實際接續：被上一列擋住，host 沒有可用的恢復通道。第二，goal-limit 停止不可誤放棄：需要進行中的目標，加上供應商回空回應。另外 11-22 的「等待中任務可見」與「立即重試」是直接呼叫已安裝的 MCP 程式驗的，沒有在面板上用滑鼠點 | 上一列做完、`controller_state=official_ipc_ready` 之後：在桌面版開一筆可拋棄任務並取得真實失敗，紀錄出現 `official_ipc_turn_started`，任務接著跑；goal-limit 停止的項目在面板沒有「放棄」按鈕（`can_dismiss=false`）；面板上對等待中的任務按一次「立即重試」，紀錄出現 `retry expedited` | Claude 主 session（agent-desktop 在 host 內操作；實體點擊前 `focus-window` 須成功，維護者在用電腦時不做；重開 Desktop 前先確認沒有進行中的對話） | [docs/DECISIONS.md](docs/DECISIONS.md)「2026-11-22」「2026-11-21」；[CHANGELOG.md:14](CHANGELOG.md)；commit `16a28f0` |

## 中期：等條件成立再做
| 項目 | 觸發條件 | 驗收方式 | 建議執行者 | 證據 |
| --- | --- | --- | --- | --- |
| `launch-codex.ps1` 的 worker 新鮮度門檻仍是固定 15 秒 | 啟用共用後端（`shared_app_server_enabled=true`）且掃描比輪詢慢的機器上，`launch-codex.ps1` 回報 `worker_not_fresh` 或略過共用後端恢復請求 | 先在隔離資料目錄重現（`status.json` 心跳 20～60 秒舊、`scan_interval_ms` 35000、worker 活著）；判斷門檻要不要跟著實測間隔放寬——那個請求只給 worker 10 秒處理，放寬門檻前要一起看請求期限；`launch-codex-smoke-test.ps1` 與 Windows gate 通過 | Codex | [scripts/launch-codex.ps1:95](scripts/launch-codex.ps1)、[scripts/launch-codex.ps1:158](scripts/launch-codex.ps1)；R-20 只修了面板、MCP 與啟動管理員 |
| 發布包含接續與面板修正的新版本 | 安裝、真實 host 與必要 smoke 驗收通過，維護者授權發布 | 同步所有版本欄位；`scripts/build-release.ps1` 產出來源雜湊一致的套件，驗證安裝及回復；release、tag、檔案皆指向已驗 SHA | Codex | [docs/DECISIONS.md:80](docs/DECISIONS.md)、[docs/DECISIONS.md:84](docs/DECISIONS.md)、[README.md:149](README.md) |
| 擴充額度時間格式 | 真實錯誤出現目前不支援的時間格式 | 去識別化錯誤先成為失敗案例，驗證日期、時區與重置後排程，Go 測試及 Windows gate 通過 | Codex | 深掃報告（2026-10-27）「24 小時制」；[scripts/source/quota_wait.go:35-36](scripts/source/quota_wait.go)（`usageLimitResetPattern` 只收 `h:mm am/pm`）；[docs/DECISIONS.md:3](docs/DECISIONS.md) |
| 重評子代理轉父任務續跑 | 假 Codex 能驗證父 thread 的 StartTurn 語意，或維護者明確接受真機風險 | 證明不對忙碌父任務重複送回合，保留原 ownership 與停止條件，測試涵蓋 owner 缺失與回復 | Claude Opus | [docs/DECISIONS.md:19](docs/DECISIONS.md)；深掃報告（2026-10-27）「刻意不做／延後」 |

## 刻意不做
| 不做什麼 | 理由 | 什麼情況下重新考慮 |
| --- | --- | --- |
| 自動重啟舊停止任務或略過關閉／需重啟的 Codex 狀態 | 使用者決定與重複回合邊界見 [docs/DECISIONS.md:19](docs/DECISIONS.md) | 維護者手動重試，或另立有可驗證語意的需求 |
| 保存供應商原始錯誤全文 | 現行面板只顯示去識別原因代碼；保存原文涉及隱私取捨 | 維護者確認保存範圍、遮罩與保留期限 |
| 為無共同祖先 merge 或重寫歷史 | 既定同步方式見 [AGENTS.md:48](AGENTS.md) | 維護者另立歷史遷移需求 |

## 維護方式
每週依 [tools/upstream_baseline.json](tools/upstream_baseline.json) 查上游差量，逐筆判斷後 cherry-pick 或最小重做；理由記 [docs/DECISIONS.md](docs/DECISIONS.md)。依賴升級須重建面板與 Go 二進位檔，檢查 build-info 與來源一致；產品變更跑 Windows gate 與產品 smoke，發布前補真實安裝及 host 驗收。
