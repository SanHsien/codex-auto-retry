@echo off
setlocal
chcp 65001 >nul
title Codex Auto Retry 緊急停用 / Safe Disable
powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -File "%~dp0startup-manager.ps1" -Action safe-disable
set "EXIT_CODE=%ERRORLEVEL%"
echo.
if not "%EXIT_CODE%"=="0" (
  echo 緊急停用失敗，請看上方的錯誤訊息。 / Safe disable failed. Review the error above.
) else (
  echo 已停用共用後端，並清除外掛自己的啟動設定。 / Shared backend disabled and plugin-owned startup state cleaned.
)
pause
exit /b %EXIT_CODE%
