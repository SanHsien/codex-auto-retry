# 路線圖

> 本 repo 後續工作的單一真相源。做完一項，就在同一個 commit 刪掉那一列；新工作只加在這裡。
> 現況段的 GitHub 數字是 2026-11-08 依 `main` `c19b1e6` 查的快照，已安裝版本是 2026-11-14 實查；開工前先重查。

## 現況
這是 Windows Codex Desktop 自動接續守護工具的維護 fork（[AGENTS.md:7](AGENTS.md)）。2026-11-08 `gh` 查詢：`c19b1e6` 的 CI、CodeQL 為 success，code scanning 開啟告警 0 筆，最近 release 為 v1.2.0；issues 停用，未查到 issue。2026-11-14 以 `scripts/install.ps1` 安裝 `966329c` 重建的 `scripts/bin`（面板技術細節保留展開），已安裝的 `%LOCALAPPDATA%\CodexAutoRetry\codex-auto-retry.exe` 與 `codex-auto-retry-mcp.exe` 的 SHA-256 都是 `D3FD195FA22C92ED153D73FDA64B28A9635F6CAE732801CCE7E97A94AB60DE2A`，等於 `scripts/build-info.json`。額度等待與面板修正仍在 Unreleased；深掃報告（2026-10-27）指出當時已安裝版尚未更新、真實 MCP host 未驗，已於 2026-11-08 核對，見上（[CHANGELOG.md:11](CHANGELOG.md)）。

## 近期：可以直接動手

| 項目 | 為什麼 | 驗收方式 | 建議執行者 | 證據 |
| --- | --- | --- | --- | --- |
| 在真實 Codex Desktop／MCP host 驗證接續與管理面板 | 已安裝版已含面板與額度等待修正，但隔離任務的接續行為與面板操作都還沒在真實 host 取得證據；驗證要在 host 視窗內開一筆可拋棄的隔離 Codex 任務，會動到執行中的守護程式與桌面。2026-11-14 已在真實 host 驗過一部分：面板開得起來、服務狀態與佇列數正確；按「重新偵測」把 11-13 已停止的舊項目列為「已停止」，沒有自動復活；對一筆舊項目按「放棄」後佇列 5→4，再重新偵測掃到 0 筆，沒有復活；技術細節原本會被定時重新整理收回，修正並安裝新版後在 host 重看，展開狀態跨多次重新整理都保留。尚未驗：重置時間顯示、對可拋棄的失敗任務按「立即重試」、goal-limit 停止不可誤放棄——這三項要有真實失敗的任務才看得到（`docs/DECISIONS.md`「2026-11-14」） | 一筆隔離任務確認重置時間、停止原因、手動重試；面板驗證狀態顯示、技術細節、立即重試與放棄；放棄後重新偵測不復活同一失敗回合，goal-limit 停止不可誤放棄；不自動復活舊停止項目 | Claude 主 session（agent-desktop 在 host 內操作；實體點擊前 `focus-window` 須成功，維護者在用電腦時不做） | [CHANGELOG.md:14](CHANGELOG.md)；commit `16a28f0`；[docs/DECISIONS.md:19](docs/DECISIONS.md)；深掃報告（2026-10-27） |

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
