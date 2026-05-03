<#
.SYNOPSIS
    Generates pkg-config files required to build Yaku's whisper.cpp CGO bindings on Windows.
.PARAMETER Prefix
    Absolute path to the whisper.cpp install root (build/whisper/install).
    Forward slashes are normalised internally.
.PARAMETER PkgConfigDir
    Directory to write the .pc files into (build/whisper/install/lib/pkgconfig).
.PARAMETER GPU
    GPU backend used when compiling whisper.cpp: "vulkan" (default) or "cpu".
#>
param(
    [Parameter(Mandatory)][string]$Prefix,
    [Parameter(Mandatory)][string]$PkgConfigDir,
    [string]$GPU = "vulkan"
)

$Prefix = $Prefix -replace '\\', '/'

$libwhisper = @"
prefix=$Prefix

Name: libwhisper
Description: Whisper speech recognition library
Version: 1.8.4
Cflags: -I`${prefix}/include
"@

# GPU-specific extra libs appended to the Libs line.
$gpuLibs = switch ($GPU) {
    "vulkan" { " -l:ggml-vulkan.a -lvulkan-1" }
    "cuda"   { " -l:ggml-cuda.a"              }
    default  { ""                             }
}

# go-whisper/sys/whisper uses `#cgo windows pkg-config: libwhisper-windows`
$libwhisperWindows = @"
prefix=$Prefix

Name: libwhisper-windows
Description: Whisper speech recognition library (Windows, GPU=$GPU)
Version: 1.8.4
Libs: -L`${prefix}/lib -L`${prefix}/lib64 -lwhisper -l:ggml.a -l:ggml-base.a -l:ggml-cpu.a$gpuLibs -lgomp -lm -lstdc++
"@

New-Item -ItemType Directory -Force $PkgConfigDir | Out-Null

[System.IO.File]::WriteAllText(
    (Join-Path $PkgConfigDir 'libwhisper.pc'), $libwhisper, [System.Text.Encoding]::ASCII)
[System.IO.File]::WriteAllText(
    (Join-Path $PkgConfigDir 'libwhisper-windows.pc'), $libwhisperWindows, [System.Text.Encoding]::ASCII)

Write-Host "Wrote libwhisper.pc and libwhisper-windows.pc to $PkgConfigDir (GPU=$GPU)"
