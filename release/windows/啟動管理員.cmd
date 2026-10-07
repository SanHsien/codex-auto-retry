@echo off
setlocal
rem A PSModulePath inherited from PowerShell 7 hides Get-FileHash from Windows PowerShell 5.1.
set "PSModulePath="
wscript.exe //B //Nologo "%~dp0startup-manager.vbs"
exit /b 0
