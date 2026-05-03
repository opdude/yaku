# Porting Yaku to Windows

This document outlines the plan for supporting a Windows build for **Yaku**.

## High-Level Roadmap

1.  **Environment Setup:** Standardize the build toolchain for Windows.
2.  **Audio Backend:** Implement the WASAPI audio capture in `internal/audio`.
3.  **Cross-Platform Build System:** Refactor `Taskfile.yml` for Windows compatibility.
4.  **Dependency Portability:** Adapt `whisper.cpp` compilation for Windows.
5.  **Wails/Platform Integration:** Configure `platform_windows.go` and Wails settings.

## Detailed Technical Tasks

### 1. Environment Setup
*   **Toolchain:** Target **MinGW-w64 via MSYS2** to keep build logic similar to Linux (using `make`/`cmake`). Alternatively, use **MSVC** if direct integration with Go's `cgo` is cleaner, though this may require more complex build script adaptations.
*   **Dependencies:** Install CMake, Go, and the required C/C++ compiler suite within the chosen toolchain environment.

### 2. Audio Backend (The Primary Coding Task)
*   **Implement `internal/audio/capturer_windows.go`:**
    *   Replace the stub with a functional implementation using **WASAPI** (Windows Audio Session API).
    *   **Strategy:** Evaluate using a Go-native WASAPI wrapper (e.g., `github.com/gen2brain/malgo` or direct CGO bindings to Windows APIs).
    *   **Requirement:** Ensure the interface matches the existing `audio.Capturer` and produces the expected 500ms PCM chunks.

### 3. Build System Adaptation (`Taskfile.yml`)
*   **Pathing:** Update `Taskfile.yml` to handle Windows path conventions (`\`) versus Linux (`/`).
*   **Environment Variables:** Refactor environment variable setting. Windows CMD/PowerShell handles `PKG_CONFIG_PATH` and path modification differently than Bash.
*   **Task Logic:** Create a `windows` namespace or use platform detection within task commands to avoid platform-specific errors (e.g., `rm -rf` vs `del /f /s /q`).

### 4. Whisper.cpp Integration
*   **CGO Compilation:** The `whisper.cpp` build via CMake must be tested on Windows.
*   **Pkg-Config:** `pkg-config` support is inconsistent on Windows.
    *   *Alternative:* If `pkg-config` fails, explicitly configure CGO flags in `internal/transcribe/transcriber.go` (`#cgo LDFLAGS: ...`) to point to the built static libraries and headers in `build/whisper/install/`.

### 5. Wails and Platform Integration
*   **`cmd/yaku/platform_windows.go`:** Implement window options (translucency, frameless behavior, always-on-top) utilizing Wails' Windows-specific APIs.
*   **Wails Config:** Ensure `wails.json` and build commands correctly package the Windows executable and assets.

## Risks and Mitigations

| Risk | Mitigation |
| :--- | :--- |
| **CGO Audio Bloat/Complexity** | Prefer Go-native wrappers over manual CGO for WASAPI if possible, unless performance is strictly inadequate. |
| **Build Tooling Fragmentation** | Strictly enforce the use of `task` to abstract the underlying platform differences (CMD/PowerShell/Bash) from the developer. |
| **C++ Header/Lib Paths** | Use environment variables explicitly set by the `task` engine to pass correct include/library paths to the Go compiler. |

## Recommendation
It is **highly recommended** to build on a Windows machine rather than cross-compiling, due to the complexity of CGO, Wails packaging, and native Windows audio API integration.
