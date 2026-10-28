# 路線圖

> 本 repo 後續工作的單一真相源。做完一項，就在同一個 commit 刪掉那一列；新工作只加在這裡。
> 現況段的數字是 2026-10-29 依 `main` `4fb3829b27e1` 查的快照，開工前先重查 GitHub。

## 現況
這是 Windows Codex Desktop 自動接續守護工具的維護 fork（[AGENTS.md:7](AGENTS.md)）。2026-10-29 `gh` 查詢顯示 CI、CodeQL、Dependency freshness、Upstream check 最近結果成功、開啟安全告警為 0，最近 release 為 v1.2.0；issues 停用，未查到 issue（2026-10-29 `gh` 查詢）。額度等待與面板修正仍在 Unreleased；深掃報告（2026-10-27）指出當時已安裝版尚未更新、真實 MCP host 未驗，今天的安裝狀態須另查（[CHANGELOG.md:11](CHANGELOG.md)）。

## 近期：可以直接動手
| 項目 | 為什麼 | 驗收方式 | 建議執行者 | 證據 |
| --- | --- | --- | --- | --- |
| 核對已安裝版並驗證新版接續行為 | repo 修正不會自行更新本機守護程式 | 先查已安裝 build 與有無重要重試；需要更新時依 README 建置安裝包並保留設定；以隔離任務確認重置時間、停止原因與手動重試，不自動復活舊停止項目 | 維護者 | [README.md:149](README.md)、[docs/DECISIONS.md:19](docs/DECISIONS.md)；深掃報告（2026-10-27）「維護者要做的事」 |
| 在真實 MCP host 驗證管理面板 | 靜態預覽與自動測試不能證明 host 內工具操作正常 | 一筆隔離任務驗證狀態顯示、技術細節、立即重試與放棄；放棄後重新偵測不復活同一失敗回合，goal-limit 停止不可誤放棄 | 維護者 | [CHANGELOG.md:14](CHANGELOG.md)；commit `16a28f0`；深掃報告（2026-10-27）「UI 優化」 |
| 釐清啟動 fail-open smoke 的舊失敗紀錄 | REVIEW 仍列原因未查，較新 CI 成功不能直接證明這支個別情境已驗 | 隔離設定執行 `scripts/startup-fail-open-smoke-test.ps1`，保存退出碼與失敗點；重現則補最小回歸修正，無法重現則更新 REVIEW 的環境與證據，並校正 REVIEW.md:59 的舊 Actions 停用敘述（該敘述與 2026-10-29 push 觸發的 CI success 矛盾） | Codex | [REVIEW.md:59](REVIEW.md)、[REVIEW.md:62](REVIEW.md)；深掃報告（2026-10-27）「刻意不做／延後」 |

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
