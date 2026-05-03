<#
.SYNOPSIS
    Wrapper that sets MinGW-w64 environment vars and runs a Wails command.
    Invoked by the Taskfile's build-windows and dev-windows tasks.
.PARAMETER Cmd
    Wails sub-command and extra flags (default: "dev").
.PARAMETER Mingw
    Path to the MinGW-w64 root (default: C:\msys64\mingw64).
#>
param(
    [string]$Cmd  = "dev",
    [string]$Mingw = "C:\msys64\mingw64"
)

$env:CC  = "$Mingw\bin\gcc.exe"
$env:CXX = "$Mingw\bin\g++.exe"
$env:PATH = "$Mingw\bin;$env:PATH"

Invoke-Expression "wails $Cmd"
