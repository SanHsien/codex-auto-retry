Option Explicit
Dim shell, fs, base, script, command, code
Set shell = CreateObject("WScript.Shell")
Set fs = CreateObject("Scripting.FileSystemObject")
base = fs.GetParentFolderName(WScript.ScriptFullName)
script = fs.BuildPath(base, "payload\codex-auto-retry\scripts\launch-codex.ps1")
If Not fs.FileExists(script) Then script = fs.BuildPath(fs.GetParentFolderName(fs.GetParentFolderName(base)), "scripts\launch-codex.ps1")
If Not fs.FileExists(script) Then
    MsgBox ChrW(25214) & ChrW(19981) & ChrW(21040) & ChrW(23433) & ChrW(20840) & ChrW(21855) & ChrW(21205) & ChrW(31243) & ChrW(24335) & ChrW(65292) & ChrW(35531) & ChrW(20808) & ChrW(23436) & ChrW(25972) & ChrW(35299) & ChrW(22739) & ChrW(32302) & ChrW(30332) & ChrW(20296) & ChrW(27284) & ChrW(65292) & ChrW(25110) & ChrW(29992) & ChrW(22519) & ChrW(34892) & ChrW(27284) & ChrW(30340) & ChrW(36984) & ChrW(21934) & ChrW(38283) & ChrW(21855) & ChrW(12290), 48, "Codex " & ChrW(23433) & ChrW(20840) & ChrW(21855) & ChrW(21205)
    WScript.Quit 1
End If
command = Quote(shell.ExpandEnvironmentStrings("%WINDIR%\System32\WindowsPowerShell\v1.0\powershell.exe")) & " -NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -WindowStyle Hidden -File " & Quote(script)
code = shell.Run(command, 0, True)
WScript.Quit code
Function Quote(value)
    Quote = Chr(34) & Replace(value, Chr(34), Chr(34) & Chr(34)) & Chr(34)
End Function
