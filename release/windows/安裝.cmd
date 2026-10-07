@echo off
setlocal
chcp 65001 >nul
rem A PSModulePath inherited from PowerShell 7 hides Get-FileHash from Windows PowerShell 5.1.
set "PSModulePath="
title Codex Auto Retry 安裝 / Installer
echo.
echo Codex Auto Retry - 一鍵安裝 / one-click installer
echo.
powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -File "%~dp0deploy.ps1" -WaitForCodexExit
set "EXIT_CODE=%ERRORLEVEL%"
echo.
if "%EXIT_CODE%"=="2" (
  echo 已取消安裝，外掛與執行環境都沒有變更。 / Installation cancelled. No plugin or runtime changes were made.
) else if not "%EXIT_CODE%"=="0" (
  echo 安裝失敗，請看上方的錯誤訊息。 / Installation failed. Review the error above.
) else (
  echo 安裝完成。 / Installation succeeded.
)
echo.
pause
exit /b %EXIT_CODE%
