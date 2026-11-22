# 路線圖

> 本 repo 後續工作的單一真相源。做完一項，就在同一個 commit 刪掉那一列；新工作只加在這裡。
> 現況段的 GitHub 數字是 2026-11-08 依 `main` `c19b1e6` 查的快照，已安裝版本與 `scripts/build-info.json` 是 2026-11-22 實查；開工前先重查。

## 現況
這是 Windows Codex Desktop 自動接續守護工具的維護 fork（[AGENTS.md:7](AGENTS.md)）。2026-11-08 `gh` 查詢：`c19b1e6` 的 CI、CodeQL 為 success，code scanning 開啟告警 0 筆，最近 release 為 v1.2.0；issues 停用，未查到 issue。2026-11-14 以 `scripts/install.ps1` 安裝 `966329c` 重建的 `scripts/bin`（面板技術細節保留展開），已安裝的 `%LOCALAPPDATA%\CodexAutoRetry\codex-auto-retry.exe` 與 `codex-auto-retry-mcp.exe` 的 SHA-256 都是 `D3FD195FA22C92ED153D73FDA64B28A9635F6CAE732801CCE7E97A94AB60DE2A`，等於當時的 `scripts/build-info.json`。2026-11-21 修好 R-20、R-21 後重建，2026-11-22 完全關閉 Codex Desktop 後以 `scripts/install.ps1` 裝上：兩個已安裝 exe 的 SHA-256 都是 `1E849611EF072EE00F3854A73B9BBD6DD90703342CEB99D064966F9A337E95EA`，等於 `scripts/build-info.json`，`status.json` 的 `build_source_hash` 等於 build-info 的 `source_hash`（`82C69F56…58F3`）。同日用 `scripts/launch-codex.ps1` 重開 Codex Desktop（`OpenAI.Codex_26.1117.4957.0`）後，`recovery_mode` 仍是 `none`。同日查實原因：Desktop `26.1117` 起不再提供官方 IPC 路由，app-server 也改由 NodeService host 啟動；傳輸判斷已修（R-22），同日又改成原生查詢、一次約 30 毫秒（R-23），已重建 `scripts/bin`，但還沒安裝，見下表前兩列。額度等待與面板修正仍在 Unreleased；深掃報告（2026-10-27）指出當時已安裝版尚未更新、真實 MCP host 未驗，已於 2026-11-08 核對，見上（[CHANGELOG.md:11](CHANGELOG.md)）。

## 近期：可以直接動手

| 項目 | 為什麼 | 驗收方式 | 建議執行者 | 證據 |
| --- | --- | --- | --- | --- |
| 安裝 R-22／R-23 修正版（傳輸判斷認得 app-server host、改走原生查詢） | 2026-11-22 已重建 `scripts/bin`（含 R-23），但安裝要先完全關閉 Codex Desktop，當時 Desktop 開著，沒有裝；維護者決定跟共用後端啟用（下一列）一起裝。已安裝版仍是 `1E849611…95EA`，對 Desktop `26.1117` 的行程樹回 `unknown`，傳輸判斷仍是一次 5～15 秒的 PowerShell | 確認沒有進行中的任務後關閉 Desktop（做法見 `docs/DECISIONS.md`「2026-11-22：R-20／R-21 修正版裝上真實 host」），`scripts\install.ps1` exit 0，兩個已安裝 exe 的 SHA-256 等於 `scripts/build-info.json`，`status.json` 的 `build_source_hash` 等於 build-info 的 `source_hash`。裝完 `recovery_mode` 仍會是 `none`（見下一列），這不算失敗；`controller_state` 不再在 `controller_timeout` 與其他值之間跳 | Claude 主 session（維護者不在用電腦、Desktop 沒有進行中對話時） | [docs/DECISIONS.md](docs/DECISIONS.md)「2026-11-22：Desktop 26.1117 起沒有官方 IPC 路由」與「2026-11-22：傳輸判斷改走原生查詢」；[scripts/build-info.json](scripts/build-info.json) |
| 決定 Desktop `26.1117` 以後怎麼接續：共用後端，或明講只能監看 | 2026-11-22 查實：`26.1117.3838.0` 起 Desktop 不再建立 `\\.\pipe\codex-ipc`，程式裡也沒有 owner 路由的協定了，官方 IPC 這條路在新版不存在。剩下的恢復路徑只有共用後端。新版程式仍讀 `CODEX_APP_SERVER_WS_URL`，但 app-server 改由 NodeService host 啟動後，共用連線是不是由 host 發出、接續是否成立，都沒有實測；R-22 只依紀錄把 host 加進可接受的用戶端 | 在不影響維護者對話的環境（另一個 Windows 帳號或測試機）用 `scripts\launch-codex.ps1` 以共用模式開 `26.1117` 以上的 Desktop：`Get-NetTCPConnection` 看到連到共用埠的行程是哪一個，`controller_state=ready`，一筆可拋棄任務的真實失敗能接著跑。成立：寫進 `docs/shared-backend-safety.md`，再決定要不要建議維護者開共用後端。不成立：README、面板文字與 `docs/shared-backend-safety.md` 改成「`26.1117` 以後只能監看與列出故障」 | Claude 主 session（先讀 `docs/shared-backend-safety.md`；不要為了驗收直接對維護者的 Desktop 開共用後端） | [docs/DECISIONS.md](docs/DECISIONS.md)「2026-11-22：Desktop 26.1117 起沒有官方 IPC 路由」；[scripts/source/desktop_transport_windows.go](scripts/source/desktop_transport_windows.go)；[docs/shared-backend-safety.md](docs/shared-backend-safety.md) |
| 在真實 Codex Desktop／MCP host 驗證實際接續與 goal-limit 停止 | 其餘面板與 MCP 操作已在 2026-11-14、11-21、11-22 三次真實 host 驗收完成，細節見 `docs/DECISIONS.md`。還沒驗兩件事。第一，實際接續：被「決定 Desktop `26.1117` 以後怎麼接續」那一列擋住，host 沒有可用的恢復通道。第二，goal-limit 停止不可誤放棄：需要進行中的目標，加上供應商回空回應。另外 11-22 的「等待中任務可見」與「立即重試」是直接呼叫已安裝的 MCP 程式驗的，沒有在面板上用滑鼠點 | host 有可用的恢復通道之後（`26.1117` 以上是 `controller_state=ready`，舊版是 `official_ipc_ready`）：在桌面版開一筆可拋棄任務並取得真實失敗，紀錄出現派送成功（舊版是 `official_ipc_turn_started`），任務接著跑；goal-limit 停止的項目在面板沒有「放棄」按鈕（`can_dismiss=false`）；面板上對等待中的任務按一次「立即重試」，紀錄出現 `retry expedited` | Claude 主 session（agent-desktop 在 host 內操作；實體點擊前 `focus-window` 須成功，維護者在用電腦時不做；重開 Desktop 前先確認沒有進行中的對話） | [docs/DECISIONS.md](docs/DECISIONS.md)「2026-11-22」「2026-11-21」；[CHANGELOG.md:14](CHANGELOG.md)；commit `16a28f0` |

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
