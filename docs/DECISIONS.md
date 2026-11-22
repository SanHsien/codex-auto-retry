# 維護決策

## 2026-11-22：Desktop 26.1117 起沒有官方 IPC 路由；傳輸判斷改認 app-server host（R-22）

接續下一段「恢復通道仍不可用」的兩條線索。全程唯讀：沒有重開或關閉 Codex Desktop，沒有改它的設定與後端路由，沒有碰對話。

- **線索一，`\\.\pipe\codex-ipc` 不存在：這版 Desktop 已經沒有那個路由，不是改名，也不是要某個條件才建立。**
  - Desktop 自己的紀錄（`%LOCALAPPDATA%\Packages\OpenAI.Codex_2p2nqsd0c76g0\LocalCache\Local\Codex\Logs`）：`26.1111.2228.0` 與 `26.1111.9664.0` 的工作階段有 `[IpcRouter] I am the router`，最後一筆是 2026-11-20T10:04:23Z。`26.1117.3838.0`（第一筆紀錄 2026-11-20T10:06:16Z）與 `26.1117.4957.0` 的工作階段一筆都沒有，改成出現 `[app-server-host] App server utility started pid=…`。所以是 `26.1111.9664.0` → `26.1117.3838.0` 這次更新拿掉的。
  - `26.1117.4957.0` 的 `app.asar`（15,520 個檔，逐檔讀內容，另外解開 68 個 `.codex-gzip`）、`app.asar.unpacked`、`resources\plugins`、`resources\native`：`thread-owner-discovery`、`thread-follower-start-turn`、`handledByClientId` 都是 0 筆。`codex-ipc` 這個字串只剩在一個算路徑的函式裡，唯一的呼叫端是 Node 權限允許清單，沒有任何程式對它 `listen`。
  - `codex.exe`（`codex-cli 0.171.0-alpha.13`）裡的 `\\.\pipe\codex-ipc` 旁邊是 `client-discovery-response`、`timed out waiting for IDE context`：它是去連 IDE 擴充套件的用戶端，不是建立管道的那一方。
  - 目前存在的管道是 `codex-app-tools-<uuid>`、`codex-browser-use-<uuid>`、`codex-computer-use-<uuid>`、`OpenAI.CodexSandbox.…`，名稱每次隨機，是 Desktop 給自己的 MCP 與瀏覽器工具用的，沒有證據顯示能用來對某個 thread 送出接續。
  - **決定**：不改 `codex_ipc_windows.go`。沒有新通道可以接，硬接任何一個隨機名稱的管道都是猜。舊版 Desktop 仍走原本的路由，所以那段程式留著。
  - **還不知道的**：新版把「誰擁有這個 thread」的協調移到哪裡（程式裡看得到行程內的 `clientCoordination`，沒有看到對外介面）；OpenAI 之後會不會再提供對外路由。
- **線索二，行程樹：已修。** 實查這台機器：主行程 `ChatGPT.exe` 55000 → `ChatGPT.exe --type=utility --utility-sub-type=node.mojom.NodeService` 13328 → `codex.exe … app-server` 48584（沒有 `--listen`）。`desktopTransportScript` 原本只認主行程的直接子行程，所以回 `unknown`。
  - 改法：一個行程要同時符合三件事才算 app-server host——父行程是已驗證的主行程、執行檔路徑跟那個父行程一模一樣、命令列有 `--type=utility` 與 `--utility-sub-type=node.mojom.NodeService`。host 底下的 `codex.exe app-server` 才算 Desktop 的 app-server。沒有選「往上走任意層直到主行程」：那會把任務或 CLI 自己開的 app-server 也算進來。
  - 同一個 host 也加進「共用連線的用戶端」名單。依據是 Desktop 紀錄裡 `[AppServerConnection] Initializing app-server transport` 由 host 那個行程寫出，而且新版程式仍讀 `CODEX_APP_SERVER_WS_URL`；這一點沒有在開著共用後端的 Desktop 上實測，見下方「沒有驗到」。
  - 測試（`scripts/source/desktop_transport_tree_windows_test.go`，資料在 `scripts/source/testdata/`）：新版行程樹是這台機器實際抓的（去掉使用者名稱與多餘參數）；舊版那份是依舊判斷的前提重建的，不是實抓，檔內有註明。未修的腳本對新版行程樹回 `unknown`，測試先紅後綠。另有 10 個不該被接受的情形（別的程式的 NodeService、不是主行程子行程的 NodeService、父行程是 renderer／NetworkService／exec-server／`node.exe`、獨立 CLI 的 app-server 等）都維持 `unknown`。
- **在這台機器上的結果（唯讀）**：
  - 直接跑傳輸判斷腳本：舊腳本 `unknown`，新腳本 `legacy_stdio`。
  - 用新程式碼跑正式的判斷流程（設定檔複製到暫存目錄，不碰已安裝的資料目錄）：傳輸 `legacy_stdio`、官方 IPC 不可用、共用後端關閉 → `controller_state` 仍是 `shared_app_server_disabled`，`recovery_mode` 仍是 `none`。
  - 也就是說，這次修正讓判斷變正確，但**不會**讓這台機器進入恢復模式。Desktop `26.1117` 以後，唯一剩下的恢復路徑是共用後端（`shared_websocket`）。
- **沒有驗到**：
  - 新版 Desktop 開共用後端時，連到共用埠的是不是 host 那個行程。要驗就得改維護者 Desktop 的後端路由，本輪不做，列在 `ROADMAP.md`。
  - 新版沒有安裝。安裝要先關 Desktop，本輪不關；已重建 `scripts/bin`，等下一次 Desktop 閒置時再裝。
- **順帶量到的**：傳輸判斷腳本在這台機器跑一次要 5～15 秒（新舊腳本交替各跑 5 次：舊 13.4、13.9、15.0、8.5、5.1 秒，新 9.7、15.4、5.2、5.8、5.9 秒），上限是 20 秒（`desktopTransportCheckTimeout`）；本輪用正式流程跑了 4 次判斷，逾時 3 次，其中 2 次發生在同時編譯 Go 測試的時候。單獨量，`Get-NetTCPConnection -State Established` 是 2～7 秒、`Get-CimInstance Win32_Process` 是 1～2 秒，其餘是 PowerShell 啟動與載入模組。新舊腳本的篩選部分在同一份行程快照上各約 0.2～0.3 秒，快慢不是這次改動造成的。11-21 看到 `controller_state` 在 `shared_app_server_disabled` 與 `controller_timeout` 之間跳，可能是同一件事，沒有另外查證；已列入 `ROADMAP.md`。

## 2026-11-22：R-20／R-21 修正版裝上真實 host；重開 Codex Desktop 後恢復通道仍不可用，實際接續沒有驗到

- **安裝**：關閉 Codex Desktop 前先確認沒有進行中的任務（`status.json` 的 `pending_retries=0`、`active_retries=0`，Codex home 的 rollout 超過 3 小時沒有寫入，沒有 `codex exec` 行程）。安裝前守護程式是 supervisor 28180、worker 30920，狀態檔裡有 109 筆舊停止紀錄。關閉分兩步：agent-desktop `close-app`（送出視窗關閉）只把視窗收掉，Desktop 留在背景；再用 Windows Restart Manager 的 `RmShutdown`（不帶強制旗標）要求結束，主行程隨即結束，其餘子行程約 9 分鐘內自己退出，沒有強制結束任何行程。`pwsh -NoProfile -File scripts\install.ps1` exit 0；兩個已安裝 exe 的 SHA-256 由 `D3FD195F…DE2A` 變成 `1E849611EF072EE00F3854A73B9BBD6DD90703342CEB99D064966F9A337E95EA`，等於 `scripts/build-info.json`；`status.json` 的 `build_source_hash` 是 `82C69F56…58F3`（等於 build-info 的 `source_hash`），並出現 `scan_interval_ms`。安裝後是 supervisor 42524、worker 29000，舊停止紀錄沒有被重新排程。
- **重開**：`scripts\launch-codex.ps1` exit 0，`desktop-launch.json` 寫 `mode=official`、`reason=shared_disabled`、`outcome=launch_requested`。`scripts\status.ps1` 回 `ProcessRunning=True`、`HeartbeatStale=False`；直接以 stdio 呼叫已安裝的 `codex-auto-retry-mcp.exe mcp` 的 `get_auto_retry_status`，回 `running=true`、`heartbeat_stale=false`。
- **沒有通過：恢復模式**。重開後觀察 5 分鐘，約 5 小時後再看一次，`recovery_mode` 都是 `none`、`controller_state=shared_app_server_disabled`。11-21 寫的推測「Desktop 不是由 `launch-codex.ps1` 開的」不成立：這次確實由它開，結果一樣。這台機器現在的 Desktop 是 `OpenAI.Codex_26.1117.4957.0`，實查到兩件事：
  - `\\.\pipe\codex-ipc` 不存在（列舉具名管道查的；Desktop 自己的 `codex-mcp-*`、`codex-app-tools-*` 等管道都在）。`dialCodexIPC` 開不到，`officialIPCReady` 就是 false。Desktop 的程式裡還留著這個管道路徑的字串，為什麼沒有建立沒有查到。
  - `codex.exe app-server` 的父行程是 `ChatGPT.exe --type=utility --utility-sub-type=node.mojom.NodeService`，不是主行程；執行檔也改從 `%LOCALAPPDATA%\OpenAI\Codex\bin\<hash>\codex.exe` 起。`desktopTransportScript` 只把主行程的直接子行程算成 app-server，所以回 `unknown`，`preflight` 連官方 IPC 都不會去試，直接落到 `shared_app_server_disabled`。就算管道回來，這個判斷也要先改。
  - 2026-11-17 的守護紀錄還有 `retry action started`、`retry rescheduled … reason=codex_thread_owner_unavailable`，代表當時管道連得上；是哪一版 Desktop 之後變的沒有查到。
- **決定**：不為了驗收對維護者的 Desktop 開共用後端模式。那會改掉 Desktop 的後端路由，而且同一個傳輸判斷只看主行程與 NetworkService 的連線，app-server 改由 NodeService 啟動後能不能認出共用連線還沒有證據，要先在隔離環境看。後續列在 `ROADMAP.md` 近期第一列與 `REVIEW.md` 未解風險。
- **新版在真實 host 上驗到的**（做法同 11-21：`codex exec` 加單次 `-c` 覆寫，把 model provider 指到本機 stub `127.0.0.1:48731`，stub 一律回 503；三次請求都打在 stub，沒有送 OpenAI 請求）：
  - 等待中的任務看得到（R-20）：`retry scheduled thread=01a1ffe7... category=server … delay_seconds=15` 之後，`get_auto_retry_status` 回 `pending_retries=1`，該筆 `state=pending`、`can_retry_now=true`。11-21 同一個情境按 12 次重新整理都看不到。
  - 對等待中的任務「立即重試」：第二筆（`01a1ffe8`）還在等待時呼叫 `retry_now`，回「已請求立即重試」，紀錄寫 `retry expedited thread=01a1ffe8...`，同一秒停在派送閘門（`reason=shared_app_server_disabled`）。請求到被處理隔了 11 秒，那一輪掃描剛好比較久。
  - 停止後看得到、可放棄：兩筆都以 `state=stopped`、`stop_reason=shared_app_server_disabled`、`can_dismiss=true` 列出；`dismiss_retry` 後紀錄寫 `retry dismissed … reason=user_control`，`rescan_interrupted_tasks` 寫 `listed=0 candidates=5`，佇列是空的。
  - 這三項是直接呼叫已安裝的 MCP 程式驗的，沒有在面板上用滑鼠點；面板與 MCP 走同一套工具，但畫面沒有重看。
- **沒有驗到**：實際接續（紀錄出現 `official_ipc_turn_started`、任務接著跑）；goal-limit 停止不可誤放棄。R-21（等額度超過 1 小時才停止的任務）沒有在真實 host 重現，要等一小時以上，只有 Go 測試。
- **收尾**：三筆可拋棄 thread 已封存（`01a1ffe7-ef58-7522-b11b-54fcc4c6c1e2`、`01a1ffe8-8414-75d0-88a7-c43ec7de1ae7`，以及 11-21 留下的 `01a1fbfa-d7c6-7c72-8cb1-989ce0684e1b`）；stub 已停；Codex Desktop 留著，是由 `launch-codex.ps1` 開的那一個。

## 2026-11-21：心跳逾時改用實測間隔、`last_scan_at` 倒退的根因、額度等待後停止的任務照常顯示（R-20、R-21）

- **心跳逾時（R-20）**：原規則是「漏掉 4 次心跳（至少 15 秒）就算失效」，但「一次心跳」被寫死成輪詢間隔。輪詢只決定兩輪之間最少隔多久；一輪掃描跑得比輪詢久時，下一輪要等它跑完（`time.Ticker` 會丟掉趕不上的 tick），真正的心跳週期是掃描本身的長度。所以保留「4 次、至少 15 秒」，只把週期改成實測值：worker 每輪記下與上一次心跳的間隔，寫進 `status.json` 的 `scan_interval_ms`；`heartbeatStaleAfter`（`scripts/source/management.go`）取 `max(輪詢, scan_interval_ms)` 的 4 倍。啟動管理員（`scripts/startup-manager.ps1` 的 `Get-HeartbeatMaxAge`）用同一條規則。沒選的做法：從 `lifecycleProbeTimeout` 推上限——一輪裡有好幾個探測，加總不是固定值；只看 worker 存活——卡死但沒結束的 worker 會被當成正常。舊版 worker 沒有這個欄位，就退回輪詢間隔，跟以前一樣。
- **`last_scan_at` 倒退**：根因是 `daemon.run` 把 `ticker.C` 收到的時間當成「現在」。Ticker 送的是排定的 tick 時間；前一輪掃描超過輪詢間隔時，排著的那個值已經落後了「超出的時間」，心跳又寫成「那個值＋本輪耗時」，所以長的一輪後面接短的一輪，心跳就往回跳。證據：同樣寫法的獨立 Go 程式（輪詢 200 ms、交替 700／50 ms 工作）每次長輪後都倒退 450 ms；本機已安裝版 11-21 04:23～04:26Z 取樣，`last_scan_at` 倒退四次（-3.9、-2.3、-2.4、-1.0 秒），每次都緊接在一輪 5～6 秒的掃描後面。修正：`run` 改用 `time.Now()`。這個落差也讓心跳看起來比實際舊，所以跟 R-20 一起修。
- **額度等待後停止的任務（R-21）**：`stoppedRetryIsVisible` 有兩道條件：停止後 1 小時內（手動停止 24 小時），而且故障本身也不能超過同一個時限。第二道是用來藏「watchdog 停機期間留下、重新啟動才看到並直接停止」的舊故障（`daemon.go` 處理事件時刻意保留停機期間的故障）。等額度是 watchdog 自己把故障留到重置時間（最長 8 天，`quotaResetHorizon`），那段時間故障並沒有變舊。所以第二道條件改從 `max(FailedAt, QuotaResetAt)` 起算，第一道不變。沒選「全部只看 `StoppedAt`」：那會讓上述停機期間的舊故障重新出現在面板。
- **驗證**：先紅後綠的 Go 測試在 `scripts/source/heartbeat_visibility_test.go`；啟動管理員的情境加在 `scripts/startup-manager-smoke-test.ps1`。

## 2026-11-21：真實 host 驗收額度重置顯示、立即重試與放棄

- **做法**：已安裝版仍是 11-14 那版（兩個 exe 的 SHA-256 `D3FD195F…DE2A`），守護程式（supervisor 28180、worker 30920）全程沒有重啟。用 Codex CLI `codex exec` 開兩筆可拋棄任務，以單次 `-c` 覆寫把 model provider 指到本機 stub（`127.0.0.1:48731`，`request_max_retries=0`），不改 `config.toml`、不送 OpenAI 請求、不耗額度。stub 第一次回 HTTP 429 `usage_limit_reached`，Codex 自己寫出「…or try again at 11:16 AM.」；第二次回 503，Codex 寫出 `unexpected status 503 Service Unavailable`。面板在 Codex Desktop（`OpenAI.Codex_26.1117.3838.0`）新開一則對話叫出，用 agent-desktop 讀無障礙樹；實體點擊前都確認 `focus-window` 成功、Codex 在前景、電腦閒置超過 60 秒。維護者的真實對話與 11-13 以前的 109 筆舊停止紀錄都沒有動。
- **通過**：
  - 重置時間：守護紀錄寫 `retry scheduled thread=01a1fbc7... category=rate_limit ... delay_seconds=5422`，`quota_reset_at` 為 `2026-11-21T11:16:00+08:00`。面板顯示「額度用完，預計 11:16 恢復；恢復後自動重試，不會先消耗次數。」與「21 分 45 秒 後重試」，技術細節多出「額度恢復 11:16」。
  - 停止原因：到期時寫 `retry stopped before dispatch ... reason=shared_app_server_disabled`；503 那筆面板顯示「已停止／共用後端已關閉」與說明文字。
  - 對已停止的可拋棄任務按「立即重試（重新計數）」：紀錄寫 `retry restarted thread=01a1fc24...`，同一秒又停在派送閘門（原因同上）。
  - 放棄：兩筆都寫 `retry dismissed ... reason=user_control`，面板佇列變空。
  - 放棄後重新偵測不復活：`interrupted task rescan completed listed=0 candidates=3`。
  - 舊停止項目不自動復活：兩次重新偵測只列出本輪自己的任務，109 筆舊紀錄沒有被重新排程。
- **沒有驗到**：
  - 實際接續：本機 Codex Desktop 目前不在可恢復模式（`controller_state` 在 `shared_app_server_disabled` 與 `controller_timeout` 之間跳，`recovery_mode=none`），所有重試都停在派送前。CLI 任務本來也不歸桌面版所有。要驗接續，需要用 `scripts/launch-codex.ps1` 重開 Codex Desktop，再拿到一筆桌面版自己的失敗任務；本輪沒有重開維護者的 Desktop。
  - 對等待中的任務按「立即重試」：面板把它藏起來了，見下方缺陷一。
  - goal-limit 停止不可誤放棄：需要進行中的 Codex 目標，加上供應商連續回空回應，stub 接不進桌面版的對話，所以沒有驗。
- **發現的缺陷（本輪未修，已列入 `ROADMAP.md` 與 `REVIEW.md` R-20、R-21）**：
  - 一、心跳判斷太嚴：`management.go:132` 的逾時是 `max(4×輪詢, 15 秒)`，但這台機器的掃描間隔實測 25～50 秒（`daemon.go:176` 的註解也寫「Probes can take most of a minute on a busy machine」）。面板與 MCP 因此在 worker 活著時回報 `running=false`、`heartbeat_stale=true`，顯示「未執行／未偵測到有效心跳」，並依 `management.go:143` 藏起等待中與執行中的任務。實測：03:08～03:13 UTC 按 12 次「重新整理狀態」，那筆等待中的任務一次都沒有出現。第一次叫出面板時，Codex 也把這個狀態轉述成「目前背景服務未執行，重試佇列為空」。
  - 二、額度等待後才停止的任務看不到：`management.go:602` 的 `stoppedRetryIsVisible` 從 `FailedAt` 起算 1 小時。等額度重置超過 1 小時的任務，一停止就直接看不到。實測：`01a1fbc7` 在 01:46:07Z 失敗，03:16:41Z 停止，03:25:10Z 的快照 `stopped_retries=0`。要按「重新偵測」，它才會以「偵測到中斷」重新列出（03:39:01Z）。


**決定**：將 `scripts/source/ui/package-lock.json` 的 `hono` 由 4.13.13 升至 4.13.16，並更新下載位置與完整性雜湊。4.13.16 是以下五筆弱點的首個修補版本，符合 `@modelcontextprotocol/sdk` 的 `^4.11.4` 與 `@hono/node-server` 的 peer dependency 範圍；不變更直接依賴、不增加 override。

**處理項目**：
- [GHSA-4m76-xfj5-7975](https://github.com/advisories/GHSA-4m76-xfj5-7975)：掛載路徑可繞過掛載前綴內的 middleware。
- [GHSA-h7mg-cq63-vhpq](https://github.com/advisories/GHSA-h7mg-cq63-vhpq)：特定路由組合或編碼換行字元可略過萬用路徑 middleware。
- [GHSA-3gg2-335m-rxc6](https://github.com/advisories/GHSA-3gg2-335m-rxc6)：CSS helper 的結束標籤跳脫不足，可造成 HTML 注入。
- [GHSA-46wx-v44x-9hh2](https://github.com/advisories/GHSA-46wx-v44x-9hh2)：CSRF 防護未檢查 `text/ping` 請求。
- [GHSA-4h7h-rv7c-7m2v](https://github.com/advisories/GHSA-4h7h-rv7c-7m2v)：帶額外參數的 multipart 表單可略過欄位驗證。

**不處理項目**：無；上述五筆均採用修補版本，沒有以開發工具用途豁免。

## 2026-11-14：真實 Codex Desktop host 面板實機與技術細節展開狀態

- **做法**：在真實 Codex Desktop（已安裝 v1.2.0）開新對話叫出管理面板，用 agent-desktop 實體點擊，點擊前確認 Codex 視窗在前景；佇列裡是 11-13 被額度或供應商故障中斷、早已結束的 Codex 派工 thread。
- **通過**：面板顯示「Codex 已接入官方恢復通道」與佇列數；按「重新偵測」後守護紀錄寫 `interrupted task rescan completed listed=5`，面板把五筆列為「已停止」並提示可按「立即重試」或「放棄」，沒有自動排重試；對其中一筆按「放棄」，紀錄寫 `retry dismissed ... reason=user_control`、佇列 5→4，再重新偵測寫 `listed=0`，被放棄的那筆沒有復活。
- **發現並修正**：「技術細節」點得開（任務 ID、故障類別、停止原因碼、故障與停止時間），但面板約每 10 秒重新整理一次就把它收回；在對話內嵌檢視時，展開讓內容變高，Codex 會把對話捲到底。`renderQueue` 改成重繪前讀出已展開的完整 thread ID 並保留，任務移除後不留狀態；新增 `scripts/source/ui/tests/panel.test.mjs`，`build.ps1` 在打包前跑它。拿掉修正時「保留展開」那條測試會紅。
- **未驗**：重置時間顯示、對可拋棄失敗任務按「立即重試」、goal-limit 停止不可誤放棄。這三項需要真實的失敗任務，本輪沒有；為避免重新開動 11-13 的舊 thread，也沒有對它們按「立即重試」。
- **新版安裝後重看（同日）**：完全關閉 Codex Desktop 後跑 `scripts/install.ps1`，exit 0；兩個已安裝 exe 的 SHA-256 等於 `scripts/build-info.json`（`D3FD195F…DE2A`）。用 `scripts/launch-codex.ps1` 重開 Codex，在側欄檢視展開剩下那筆已停止項目（`01a1d4ba`）的「技術細節」（展開前「掃描於」為 15:45:19、收合）。展開後第一張截圖的「掃描於」是 15:46:05，約三分鐘後再截，「掃描於」已到 15:49:21，技術細節仍是展開，期間面板重新整理多次。對話內嵌檢視仍會在內容變高時被 Codex 捲到底，那是 host 行為；要看細節請按 ↗ 改用側欄檢視。

## 2026-10-26：額度用完時等到重置再試，控制端暫時連不上不放棄

**事件**：Codex 額度用完（`rate_limit`），重試程式 00:53（UTC）對 `01a0b950` 送出續跑並被接受；之後 01:03～01:05 的生命週期探測連續 `controller_unavailable`，到 `controller_failure_limit`（3）就 `retry stopped`，同時三個子代理 thread 因 `codex_thread_owner_unavailable` 停掉。額度恢復後沒有人再叫醒 Codex。

**根因**：`controller_unavailable` 是「沒有歸類的錯誤」的預設名稱。實際來源是判斷桌面版傳輸方式的 PowerShell 檢查（`desktop_transport_windows.go`）在機器忙碌時超過 8 秒被砍；生命週期探測自己的預算只有 5 秒，比它包住的檢查還短。`shared_app_server_enabled=false` 是官方 IPC 模式的正常狀態，與此無關。

**決定**：
- 傳輸檢查逾時放寬為 20 秒（`desktopTransportCheckTimeout`），生命週期探測預算為其加 5 秒；指令因自己的期限被砍時回報 `controller_timeout`，不再混成 `controller_unavailable`。
- 錯誤文字含「try again at …」時（`quota_wait.go`），下一次重試排在重置時間加 30 秒，30 分鐘自動恢復視窗從重置時間起算（`ThreadState.QuotaResetAt`）。解析不到時間（例如只有 429）維持原本的短間隔。
- `usage limit`、`usage_limit` 與帶 plan／billing 字樣的 `quota exceeded` 歸為 `rate_limit`，原本落到 `unknown` 只能試 3 次。
- `rate_limit` 類別的控制端失敗分三種：確定沒送出的（`codex_thread_owner_unavailable`、`codex_background_channel_unavailable`）不受 `controller_failure_limit` 限制，一律退避重試；可能已送出的（`controller_timeout`、`controller_unavailable`）上限放大為 4 倍但仍有限，避免重複開回合；其餘（例如 `start_not_acknowledged`）維持原上限。唯一的總期限是 30 分鐘恢復視窗。派送閘門遇到暫時性控制端狀態時，`rate_limit` 任務留在佇列而不是被停止；Codex 已關閉（`codex_not_running`）、需要重啟或設定衝突仍照既有約定停止並等使用者處理。
- 生命週期探測失敗改為退避（最長 5 分鐘）而非停止，超過視窗才停；探測與派送前的就緒檢查預算皆為 45 秒（傳輸檢查在共用模式會做兩次）。
- 「try again at」只有時分時，以錯誤發生時間為基準判斷是哪一天；重試失敗後若沒有新的重置時間，保留較晚的重置時間作為恢復視窗起點。`quota exceeded` 只在同時含 plan／billing／usage 字樣時才歸為 `rate_limit`。

**理由**：額度重置往往是數小時後，短間隔重試只會用掉次數；而重試已被接受時，探測失敗不代表任務結束。

**限制／延後**：子代理 owner 不在時只做等待，沒有改走父 thread（官方 IPC 對父 thread 開新回合的行為無法在不碰真 Codex 的情況下驗證）；已停止的舊紀錄需在新版安裝後由使用者手動按「重試」。

**觸發條件**：改動 `scheduleFailureLocked`、`rescheduleAwaitingWithPolicyLocked` 或生命週期探測時需保留 `quota_wait_test.go` 的行為；若 Codex 改變用量錯誤文字格式，更新 `usageLimitResetPattern`。

## 2026-10-26：面板打包移除 Zod 動態 Function

**決定**：`scripts/source/ui/vite.config.ts` 新增建置外掛，在打包時把 `zod/v4/core` 的 `doc.js`、`util.js`、`compile.js` 三處 `new Function` 換成停用版本（Zod 退回解譯式驗證），並在產出含 `new Function(` 時讓建置失敗。目的是消除 CodeQL 警示 `js/bad-code-sanitization`（`panel.html`）。來源比對失敗也會讓建置失敗，升級 Zod 時需重新確認。

**來源**：Codex 在 `codex-auto-retry-remediation-20261026` 的未提交改動，已在新 worktree 重現並確認產出的 `panel.html` 逐位元相同。

## 2026-10-25：修 CodeQL 首輪兩個警示

**決定**：`logger.go` 的 `Printf` 把訊息中的 CR／LF 轉成可見的 `\r`／`\n`，一筆紀錄永遠只占一行（`go/log-injection`）；`codex_ipc_windows.go` 的 `writeFrame` 比照 `readFrame` 以同一常數 `maxCodexIPCFrameBytes`（256 MiB）限制長度，再配置緩衝區（`go/allocation-size-overflow`）。

**理由**：兩處都是對上游程式的最小硬化，不改行為（正常訊息不含換行、IPC 框架遠小於上限）。已補 `TestLoggerKeepsEachRecordOnOneLine`。

**觸發條件**：上游改動這兩個檔案時，cherry-pick 需保留這兩處。

## 2026-10-25：補齊維護自動化，並分流上游 issue #1

**決定**：
- 新增 `.github/dependabot.yml`（github-actions、兩個 Go module、面板 npm、pip）、`codeql.yml`（Go 與 JavaScript/TypeScript，Go 在 Windows runner 建置）、`dependency-freshness.yml`（每月）與 `upstream-check.yml`（每週）。
- CI 的 Windows job 加跑 `tools\dev_check.ps1`，讓維護工具測試與連結檢查與本機 gate 同一組。
- 上游 issue #1（Wails 桌面客戶端的 ADR 規格）：**略過**。發文者在同一 issue 留言說明發錯 repo、屬於其自己 fork 的路線圖，且已關閉；內容是他人的產品規劃，不是缺陷回報。`reviewed_issue_through` 升為 1。
- 重寫歷史：2026-10-24 的 `Initial commit` 把本 repo 歷史壓成單一根提交，與上游不再有共同祖先。`reviewed_through` 水位比對的是 upstream remote 的 SHA，不受影響；但上游變更只能 cherry-pick 或最小重做，不能直接 merge。

**理由**：稽核缺口 `dependabot`、`codeql`、`freshness`、`upstream_check` 都是缺檔；上游檢查器若不推進 issue 水位，嚴格模式會對已分流的項目永遠紅燈。

**觸發條件**：上游 `main` 有新 commit、PR 或 issue 時，由週檢查通知後逐筆審查。

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

## 2026-10-05：設定視窗顯示任務標題，取消與中斷的任務保留給使用者決定

**決定**：設定視窗從 Codex 的 `session_index.jsonl` 讀任務標題顯示在佇列；取消等待中的重試改為「已取消」保留 24 小時；新增「重新偵測中斷任務」，把最近 24 小時最後一輪可重試失敗、之後沒再繼續的任務列為「偵測到中斷」。兩者都只能由使用者按重新開始接回。

**理由**：只顯示 ID 認不出是哪個任務；取消即刪除、又因事件已處理而不會再被掃到，使用者沒有補救方法。

**限制**：
- 標題是 Codex 依對話產生的摘要，屬於使用者內容：只在本機設定視窗讀取顯示，不寫入狀態檔或日誌，也不放進 MCP 回傳（那會進到模型的對話內容）。
- 重新偵測不處理目標模式進行中或被暫停的任務，略過已封存的任務、超過 64 MB 的會話檔與超過 24 小時的失敗；一次最多讀 512 MB。已達上限而停止、且超過一小時不再顯示的任務，也可能被重新列為「偵測到中斷」，仍要使用者按重新開始。
- 系統匣的「已停止」計數不含已取消與偵測到中斷的項目。

## 2026-10-04：版本號改為本 fork 的語意化版本線 1.0.0

**決定**：外掛、背景服務與面板版本改為 `1.0.0`，之後依 semver 遞增：介面或功能新增升 minor、只修錯升 patch、使用者需要手動處理的不相容變更升 major。上游對應版本寫在 CHANGELOG 與 release 說明；`0.7.12-fork.N` 的 release 與 tag 已刪除。

**理由**：`-fork.N` 尾碼不好讀；沿用上游 0.7.x 或 0.8.0 又會和上游未來的版本撞號。

**限制**：同步上游時版本號不跟著上游走，`scripts/build-release.ps1` 仍要求 `model.go` 的 `appVersion` 與外掛基礎版本一致；改版本要一起改 `.codex-plugin/plugin.json`、`scripts/source/model.go`、`scripts/source/ui/package.json`（含 lock）與 `panel.ts` 裡的兩處。

## 2026-10-04：整套繁中／英文雙語

**決定**：有語言設定可讀的介面（設定視窗、啟動管理員、面板、系統匣、面板回覆訊息）依 `ui-language.json` 切換；沒有機會先讀設定的地方（安裝程式、執行檔選單、`.cmd`、命令列進度、錯誤訊息、記憶體警示、關閉 Codex 提示、MCP 工具說明）一律「中文 / English」並列。

**理由**：安裝當下還沒有語言設定，並列才能保證兩種使用者都看得懂；錯誤訊息保留原英文句子，既有測試與日後搜尋英文錯誤都不受影響。

**限制**：
- 含中文的 PowerShell 腳本必須有 UTF-8 BOM，否則 Windows PowerShell 5.1 讀成亂碼；契約測試會檢查。
- 背景服務寫進日誌的訊息維持英文（給維護者看）。
- 新增使用者看得到的訊息時要同時寫中英文，契約測試會擋下只有單一語言的 `throw`／`Write-Step`。

## 2026-10-03：產品介面與入口檔名改為繁體中文

**決定**：系統匣、設定視窗、內嵌面板、MCP 工具說明、安裝提示與 `release/windows/` 入口檔名全部改為臺灣繁體中文（`安裝.cmd`、`解除安裝.cmd`、`啟動管理員.cmd`、`安全啟動Codex.vbs`、`README-安裝說明.txt`）；預設後備重試文字由「继续」改為「繼續」。轉換由 `tools/convert_zh_hant.py`（OpenCC `s2twp` 加用語對照表）完成，`test_product_strings_are_traditional_chinese` 防止回歸。

**理由**：維護者要求只留繁體中文與英文。逐字手改無法跟上上游，所以用可重跑的工具；同步上游後重跑即可。

**限制**：
- 既有使用者設定檔裡的後備重試文字不會被改寫，只有新安裝採用新預設。
- 測試裡「UTF-8 被當成 GBK」的亂碼字串必須原樣保留，工具依標記跳過。
- `assets/` 截圖由 `tools/capture_screenshots.ps1` 重拍（2026-10-03）；啟動管理員也已加上繁中介面並重拍。

## 2026-10-03：提供單檔安裝程式，不做免安裝版

**決定**：`scripts/build-release.ps1` 除了壓縮檔，另外產生單一執行檔 `Codex-Auto-Retry-<版本>-windows-x64.exe`（2026-10-03 起不帶 `-setup`，直接雙擊顯示操作選單）。它是 `scripts/installer/` 的小程式後面直接接上同一份壓縮檔；執行時把壓縮檔解到暫存資料夾，再呼叫其中的 `deploy.ps1`。

**理由**：本工具必須註冊開機啟動、Codex 外掛與背景服務，沒有「免安裝直接執行」的形態；能省的是解壓縮與找入口檔這一步。安裝邏輯仍只有 `deploy.ps1` 一份，完整性檢查與失敗回復不會出現第二套實作。

**限制**：未做程式碼簽章，SmartScreen 會提示未知發行者；自解壓執行檔也較容易被防毒軟體誤判，所以壓縮檔照常提供。

## 2026-10-02：上游檢查涵蓋 Commit、PR 與 Issue 三面向

**決定**：`check_upstream_updates.py` 以 `--state all` 收集上游 PR 與 Issue，並追蹤 Commit SHA。`gh` 失敗時 fail closed（exit 2）。

**理由**：未合併即關閉的 PR 與待處理的 Issue 同樣可能揭露重要缺陷或需求。排程報告必須確保「未檢查」與「沒有新變更」截然分明。

## 2026-10-02：日常直接推 main

**決定**：日常維護修改在本機跑 `tools\dev_check.ps1` 後直接推 `origin/main`。Dependabot 與外部貢獻仍走 PR，合併前讀 diff。

**理由**：對齊 SanHsien 體系其他維護 fork 的治理規範。

## 2026-11-07：離開 GitHub fork 網路（已執行）

- **判準**：維護者 2026-11-07 定的：與上游差異大、本線有獨有功能、已分叉獨立發展，就可以脫離；與上游有沒有共用歷史、上游活不活躍都不列入（repo-fleet-ops `docs/PLAYBOOK.md` §9）。
- **本線的獨有功能**：自己的版本線（1.x，上游 0.7.12）、只支援 Windows（移除非 Windows 程式）、GUI 安裝程式與控制中心、系統匣動態圖示、開始功能表與「已安裝的應用程式」註冊。
- **決定**：建議在 Settings → General → Danger Zone 按 Leave fork network（https://github.com/SanHsien/codex-auto-retry/settings）。不可逆，由維護者本人按。
- **來源 credit 不靠 fork 標示**：README 開頭、`NOTICE.md`、`FORK.md` 都寫明 fork 自 [`sybxxx/codex-auto-retry`](https://github.com/sybxxx/codex-auto-retry) 與授權，脫離後原樣保留。
- **上游追蹤照舊**：`tools/upstream_baseline.json` 與 `Upstream check` workflow 不依賴 fork 標示；repo-fleet-ops 的 `audit` 認水位檔，換台電腦重新 clone 也照樣要求上游檢查。
- **會遺失**：GitHub 文件說不保留 issue、PR、wiki、star、watcher。2026-11-07 查：PR 7 筆（全已結束）、star 0、wiki 沒有內容。release `v1.2.0` 是否保留文件沒寫，資產與說明已備份到維護者的 `OneDrive\文件\電腦維護\2026-11 fork 脫離前 release 備份\`（雜湊已核對）。
- **結果**：維護者 2026-11-07 已按。驗收：`gh api repos/SanHsien/codex-auto-retry --jq .fork` 回 `false`、`parent` 為空；PR 7 筆、release v1.2.0（4 個資產）都還在（GitHub 文件說 PR 不保留，實際保留）；Dependabot 告警與 Actions 照常；`main` 原本就沒有分支保護（脫離前推送時 GitHub 沒有回報 bypass），同日依 repo-fleet-ops PLAYBOOK §3 補上 `Windows validation`、`go security scan` 兩個必要檢查；repo-fleet-ops `audit` 0 缺口。

## 2026-11-08：已安裝版核對完成；真實 host 驗收列為近期（維護者）

- **核對結果**：已安裝 `codex-auto-retry.exe` 的 SHA-256 與 `scripts/build-info.json` 的 `codex-auto-retry.exe` 欄位完整相同（`4467fcf869055038609098278bf1e924f582b05f3769c21fb02f77a4a55d3805`，2026-11-08 `sha256sum` 實查）；該檔最後更新於 `16a28f0`（2026-10-26）。其後 `scripts/source` 唯一變動是 `c4337cd`（只改 `ui/package-lock.json`），安裝版因此已含面板與額度等待修正，不需為了這件事重建。
- **決定**：隔離任務的接續行為驗證與面板操作驗證，都要動執行中的守護程式與 Codex Desktop 視窗，合併成 `ROADMAP.md` 近期的一列，執行者是Claude 主 session（agent-desktop 在 host 內操作；實體點擊前 `focus-window` 須成功，維護者在用電腦時不做）。這一列不需要登入或憑證，只是在 host 視窗開一筆可拋棄的隔離任務並操作面板，依 repo-fleet-ops `docs/PLAYBOOK.md` 的 GUI 驗收規則由 Claude 用 agent-desktop 代做；它不是等外部事件的中期列，而是現在就可以做的工作。
- **後續**：這一列驗收通過，才輪到中期「發布」列（發布 Unreleased 修正的前提）。
