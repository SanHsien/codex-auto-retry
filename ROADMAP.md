# 路線圖

> 本 repo 後續工作的單一真相源。做完一項，就在同一個 commit 刪掉那一列；新工作只加在這裡。
> 現況段的 GitHub 數字是 2026-11-08 依 `main` `c19b1e6` 查的快照，已安裝版本是 2026-11-21 實查；開工前先重查。

## 現況
這是 Windows Codex Desktop 自動接續守護工具的維護 fork（[AGENTS.md:7](AGENTS.md)）。2026-11-08 `gh` 查詢：`c19b1e6` 的 CI、CodeQL 為 success，code scanning 開啟告警 0 筆，最近 release 為 v1.2.0；issues 停用，未查到 issue。2026-11-14 以 `scripts/install.ps1` 安裝 `966329c` 重建的 `scripts/bin`（面板技術細節保留展開），已安裝的 `%LOCALAPPDATA%\CodexAutoRetry\codex-auto-retry.exe` 與 `codex-auto-retry-mcp.exe` 的 SHA-256 都是 `D3FD195FA22C92ED153D73FDA64B28A9635F6CAE732801CCE7E97A94AB60DE2A`，等於當時的 `scripts/build-info.json`。2026-11-21 再查，已安裝版雜湊不變，但 `ae5d007` 之後重建的 `scripts/build-info.json` 已是 `1359050D…6401`，已安裝版還沒裝這次重建。額度等待與面板修正仍在 Unreleased；深掃報告（2026-10-27）指出當時已安裝版尚未更新、真實 MCP host 未驗，已於 2026-11-08 核對，見上（[CHANGELOG.md:11](CHANGELOG.md)）。

## 近期：可以直接動手

| 項目 | 為什麼 | 驗收方式 | 建議執行者 | 證據 |
| --- | --- | --- | --- | --- |
| 在真實 Codex Desktop／MCP host 驗證接續與 goal-limit 停止 | 面板操作在 2026-11-14 與 2026-11-21 兩次真實 host 驗收大致完成，細節見 `docs/DECISIONS.md` 的「2026-11-21」。已驗：重置時間顯示、停止原因、技術細節、對已停止任務按「立即重試」（`retry restarted`）、放棄、放棄後重新偵測不復活、舊停止項目不自動復活。還沒驗三件事。第一，實際接續：11-21 時 host 的 `recovery_mode=none`，因為 Codex Desktop 不是由 `scripts/launch-codex.ps1` 開的，所有重試都停在派送前。第二，對等待中的任務按「立即重試」：被下一列的心跳缺陷擋住。第三，goal-limit 停止不可誤放棄：需要進行中的目標，加上供應商回空回應 | 用 `scripts/launch-codex.ps1` 重開 Codex Desktop，確認 `controller_state=official_ipc_ready`；在桌面版開一筆可拋棄任務並取得真實失敗，按「立即重試」後，紀錄出現 `official_ipc_turn_started`，任務接著跑；goal-limit 停止的項目在面板沒有「放棄」按鈕（`can_dismiss=false`） | Claude 主 session（agent-desktop 在 host 內操作；實體點擊前 `focus-window` 須成功，維護者在用電腦時不做；重開 Desktop 前先確認沒有進行中的對話） | [docs/DECISIONS.md](docs/DECISIONS.md)「2026-11-21」；[CHANGELOG.md:14](CHANGELOG.md)；commit `16a28f0` |
| 心跳逾時太短，面板在守護程式正常時顯示「未執行」並藏起等待中任務 | `management.go:132` 的逾時是 `max(4×poll, 15s)`；`daemon.go:176` 已承認忙碌機器上一次掃描可能將近一分鐘。2026-11-21 host 實測掃描間隔 25～50 秒，`controller_state` 在 `shared_app_server_disabled`／`controller_timeout` 之間跳，`get_auto_retry_status` 在 worker 活著時回 `running=false`、`heartbeat_stale=true`，`management.go:143` 因此藏起 pending／active 項目，按 12 次「重新整理狀態」都看不到那筆等待中的任務（REVIEW R-20） | 重現：讓 `status.json` 的 `last_scan_at` 比 now 舊 20～60 秒、worker PID 仍存活，快照不得回 `running=false`，也不得藏起 pending 項目。先補失敗測試，逾時改成依實際探測預算（`lifecycleProbeTimeout`）推算，或改用 worker 存活判斷；Go 測試、面板測試與 Windows gate 都要通過 | Codex | [docs/DECISIONS.md](docs/DECISIONS.md)「2026-11-21」缺陷一；[scripts/source/management.go:132](scripts/source/management.go)；[scripts/source/daemon.go:176](scripts/source/daemon.go) |
| 額度等待超過 1 小時後才停止的任務，在面板看不到 | `management.go:602` 的 `stoppedRetryIsVisible` 從 `FailedAt` 起算 1 小時，但額度等待可以長達 8 天（`quotaResetHorizon`）。2026-11-21 實測：`01a1fbc7` 在 01:46Z 失敗，03:16Z 停止，快照 `stopped_retries=0`，要手動按「重新偵測」才會重新列出（REVIEW R-21） | 重現：rate_limit 任務的 `FailedAt` 是兩小時前，`StoppedAt` 是現在，原因 `shared_app_server_disabled`，快照必須列出它，並可按「立即重試」與「放棄」。補失敗測試，顯示期間改從 `StoppedAt`（或額度重置時間）起算；Go 測試與 Windows gate 都要通過 | Codex | [docs/DECISIONS.md](docs/DECISIONS.md)「2026-11-21」缺陷二；[scripts/source/management.go:602](scripts/source/management.go) |

## 中期：等條件成立再做
| 項目 | 觸發條件 | 驗收方式 | 建議執行者 | 證據 |
| --- | --- | --- | --- | --- |
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
