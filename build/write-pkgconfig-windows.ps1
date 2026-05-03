<#
.SYNOPSIS
    Generates pkg-config files required to build Yaku's whisper.cpp CGO bindings on Windows.
.PARAMETER Prefix
    Absolute path to the whisper.cpp install root (build/whisper/install).
    Forward slashes are normalised internally.
.PARAMETER PkgConfigDir
    Directory to write the .pc files into (build/whisper/install/lib/pkgconfig).
#>
param(
    [Parameter(Mandatory)][string]$Prefix,
    [Parameter(Mandatory)][string]$PkgConfigDir
)

$Prefix = $Prefix -replace '\\', '/'

$libwhisper = @"
prefix=$Prefix

Name: libwhisper
Description: Whisper speech recognition library
Version: 1.8.4
Cflags: -I`${prefix}/include
"@

# go-whisper/sys/whisper uses `#cgo windows pkg-config: libwhisper-windows`
# Libs mirrors the template in go-whisper's generate.go for Windows.
$libwhisperWindows = @"
prefix=$Prefix

Name: libwhisper-windows
Description: Whisper speech recognition library (Windows, CPU)
Version: 1.8.4
Libs: -L`${prefix}/lib -L`${prefix}/lib64 -lwhisper -l:ggml.a -l:ggml-base.a -l:ggml-cpu.a -lgomp -lm -lstdc++
"@

New-Item -ItemType Directory -Force $PkgConfigDir | Out-Null

[System.IO.File]::WriteAllText(
    (Join-Path $PkgConfigDir 'libwhisper.pc'), $libwhisper, [System.Text.Encoding]::ASCII)
[System.IO.File]::WriteAllText(
    (Join-Path $PkgConfigDir 'libwhisper-windows.pc'), $libwhisperWindows, [System.Text.Encoding]::ASCII)

Write-Host "Wrote libwhisper.pc and libwhisper-windows.pc to $PkgConfigDir"
