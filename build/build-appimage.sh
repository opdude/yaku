#!/usr/bin/env bash
# Build yaku and package it as an AppImage.
# Usage: GPU=vulkan bash build/build-appimage.sh
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GPU="${GPU:-vulkan}"
VERSION="$(git -C "$ROOT" describe --tags --always --dirty 2>/dev/null || echo dev)"
OUTDIR="$ROOT/bin"
OUTFILE="$OUTDIR/yaku-${VERSION}-linux-x86_64.AppImage"
TOOLS="$ROOT/build/tools"
APPDIR="$ROOT/build/.AppDir"

LINUXDEPLOY="$TOOLS/linuxdeploy-x86_64.AppImage"
LINUXDEPLOY_GTK="$TOOLS/linuxdeploy-plugin-gtk.sh"

echo "==> yaku AppImage builder  (GPU=$GPU, version=$VERSION)"

# ── 1. Build ──────────────────────────────────────────────────────────────────
echo "==> Building Wails app..."
cd "$ROOT"
GPU="$GPU" go tool task default

BINARY="$OUTDIR/yaku"
[ -f "$BINARY" ] || { echo "ERROR: build did not produce $BINARY"; exit 1; }

# ── 2. Download linuxdeploy tools ────────────────────────────────────────────
mkdir -p "$TOOLS"
mkdir -p "$OUTDIR"

if [ ! -x "$LINUXDEPLOY" ]; then
  echo "==> Downloading linuxdeploy..."
  curl -fsSL --retry 3 -o "$LINUXDEPLOY" \
    "https://github.com/linuxdeploy/linuxdeploy/releases/download/continuous/linuxdeploy-x86_64.AppImage"
  chmod +x "$LINUXDEPLOY"
fi

if [ ! -x "$LINUXDEPLOY_GTK" ]; then
  echo "==> Downloading linuxdeploy-plugin-gtk..."
  curl -fsSL --retry 3 -o "$LINUXDEPLOY_GTK" \
    "https://raw.githubusercontent.com/linuxdeploy/linuxdeploy-plugin-gtk/master/linuxdeploy-plugin-gtk.sh"
  chmod +x "$LINUXDEPLOY_GTK"
fi

# linuxdeploy discovers plugins via PATH
export PATH="$TOOLS:$PATH"

# ── 3. Build AppDir ───────────────────────────────────────────────────────────
echo "==> Creating AppDir..."
rm -rf "$APPDIR"
mkdir -p \
  "$APPDIR/usr/bin" \
  "$APPDIR/usr/share/applications" \
  "$APPDIR/usr/share/icons/hicolor/256x256/apps"

# Main binary
cp "$BINARY" "$APPDIR/usr/bin/yaku"

# Audio capture tools (parec + pactl from PipeWire/PulseAudio)
for bin in parec pactl; do
  src="$(command -v "$bin" 2>/dev/null || true)"
  if [ -n "$src" ]; then
    echo "  bundling $bin"
    cp "$src" "$APPDIR/usr/bin/$bin"
  else
    echo "  WARNING: $bin not found — audio capture may fail on target systems without PipeWire"
  fi
done


# Desktop entry — copied into the AppDir so linuxdeploy can read it.
# Icon and AppRun are passed from OUTSIDE the AppDir so linuxdeploy places them
# correctly without creating broken self-referential symlinks.
# linuxdeploy matches the icon filename (without extension) to the desktop
# file's Icon= field. The Wails icon is named appicon.png, so copy it to a
# temp path with the correct name before passing it to linuxdeploy.
ICON_SRC="$ROOT/cmd/yaku/build/appicon.png"
ICON_NAMED="/tmp/yaku.png"
cp "$ICON_SRC" "$ICON_NAMED"
cp "$ROOT/build/yaku.desktop" "$APPDIR/usr/share/applications/"
cp "$ROOT/build/yaku.desktop" "$APPDIR/yaku.desktop"

# ── 4. Bundle shared libraries ────────────────────────────────────────────────
echo "==> Bundling shared libraries (this may take a minute)..."

export OUTPUT="$OUTFILE"
export DEPLOY_GTK_VERSION=3

# Step 4a: Let linuxdeploy + GTK plugin set up the AppDir (no AppImage yet).
# We omit --output so we can purge WebKit before packaging.
# --icon-file and --custom-apprun must point OUTSIDE the AppDir.
APPIMAGE_EXTRACT_AND_RUN=1 "$LINUXDEPLOY" \
  --appdir "$APPDIR" \
  --executable "$APPDIR/usr/bin/yaku" \
  --plugin gtk \
  --desktop-file "$APPDIR/yaku.desktop" \
  --icon-file "$ICON_NAMED" \
  --custom-apprun "$ROOT/build/AppRun"

# Step 4b: Purge libraries that must come from the system.
# Libraries bundled by linuxdeploy often have version/ABI conflicts with the
# system versions they were built against. Better to use system versions:
# - WebKit, GStreamer, GLib, libsoup-3: desktop stack (WebKit-specific)
# - libpcre2, libmount, libblkid, libuuid, libffi: system core libraries (version-critical)
echo "==> Removing bundled system libraries (using system versions at runtime)..."
find "$APPDIR" \( \
  -name "libwebkit2gtk*.so*" -o \
  -name "libjavascriptcoregtk*.so*" -o \
  -name "libsoup-3*.so*" -o \
  -name "libgst*.so*" -o \
  -name "libglib-2.0*.so*" -o \
  -name "libgobject-2.0*.so*" -o \
  -name "libgio-2.0*.so*" -o \
  -name "libpcre2-8*.so*" -o \
  -name "libmount*.so*" -o \
  -name "libblkid*.so*" -o \
  -name "libuuid*.so*" -o \
  -name "libffi*.so*" \
\) -delete

# Step 4c: Download appimagetool if not already available.
APPIMAGETOOL="$TOOLS/appimagetool-x86_64.AppImage"
if [ ! -x "$APPIMAGETOOL" ]; then
  echo "==> Downloading appimagetool..."
  curl -fsSL --retry 3 -o "$APPIMAGETOOL" \
    "https://github.com/AppImage/appimagetool/releases/download/continuous/appimagetool-x86_64.AppImage"
  chmod +x "$APPIMAGETOOL"
fi

# Step 4d: Package the AppImage.
echo "==> Creating AppImage..."
APPIMAGE_EXTRACT_AND_RUN=1 "$APPIMAGETOOL" "$APPDIR" "$OUTFILE"

echo ""
echo "==> Done: $OUTFILE"
echo "    $(du -sh "$OUTFILE" | cut -f1)  $(basename "$OUTFILE")"
