#!/bin/bash
set -euo pipefail

ARCH="${1:-amd64}"

case "$ARCH" in
  amd64|x86_64)
    GOARCH=amd64
    GNU_ARCH=x86_64
    CC=gcc
    RUNTIME_ARCH=x86_64
    ;;
  arm64|aarch64)
    GOARCH=arm64
    GNU_ARCH=aarch64
    CC=aarch64-linux-gnu-gcc
    RUNTIME_ARCH=aarch64
    ;;
  riscv64)
    GOARCH=riscv64
    GNU_ARCH=riscv64
    CC=riscv64-linux-gnu-gcc
    RUNTIME_ARCH=riscv64
    ;;
  *)
    echo "Usage: $0 {amd64|arm64|riscv64}" >&2
    exit 1
    ;;
esac

PROJECT_ROOT="$(cd "$(dirname "$0")" && pwd)"
BUILD_DIR="${PROJECT_ROOT}/build-appimage-${ARCH}"
APPDIR="${BUILD_DIR}/AppDir"
DIST_DIR="${PROJECT_ROOT}/dist"
mkdir -p "$APPDIR/usr/bin" "$APPDIR/usr/lib" "$DIST_DIR"
mkdir -p "$APPDIR/usr/share/applications"
mkdir -p "$APPDIR/usr/share/icons/hicolor/256x256/apps"

cd "$PROJECT_ROOT"
make icons

echo "[1/6] Building nozik for linux/${GOARCH}"
GOOS=linux GOARCH="$GOARCH" CGO_ENABLED=1 CC="$CC" \
  go build -ldflags="-s -w" -o "${APPDIR}/usr/bin/nozik" ./cmd/nozik

READELF="${GNU_ARCH}-linux-gnu-readelf"
if ! command -v "$READELF" >/dev/null 2>&1; then
  READELF=readelf
fi

copy_needed_libs() {
  local binary="$1"
  local needed
  needed=$("$READELF" -d "$binary" 2>/dev/null | while read -r line; do
    case "$line" in
      *"(NEEDED)"*) echo "$line" | cut -d'[' -f2 | cut -d']' -f1 ;;
    esac
  done)

  local lib prefix found
  for lib in $needed; do
    case "$lib" in
      libc.so*|libpthread.so*|libdl.so*|librt.so*|libm.so*|ld-linux*.so*|ld64.so*|linux-vdso.so*)
        continue
        ;;
    esac
    if [ -f "${APPDIR}/usr/lib/${lib}" ]; then
      continue
    fi
    found=0
    for prefix in "/usr/lib/${GNU_ARCH}-linux-gnu" "/usr/${GNU_ARCH}-linux-gnu/lib" "/usr/local/lib/${GNU_ARCH}-linux-gnu"; do
      if [ -f "${prefix}/${lib}" ]; then
        cp -L "${prefix}/${lib}" "${APPDIR}/usr/lib/"
        found=1
        break
      fi
    done
    if [ "$found" = 0 ]; then
      echo "WARNING: $lib not found for $ARCH (required by $binary)" >&2
    fi
  done
}

echo "[2/6] Bundling shared libraries"
copy_needed_libs "${APPDIR}/usr/bin/nozik"
prev_count=-1
while true; do
  for lib in "${APPDIR}/usr/lib/"*.so*; do
    [ -f "$lib" ] || continue
    copy_needed_libs "$lib"
  done
  new_count=$(ls -1 "${APPDIR}/usr/lib/"*.so* 2>/dev/null | wc -l)
  if [ "$prev_count" = "$new_count" ]; then
    break
  fi
  prev_count=$new_count
done

echo "[3/6] Bundling gdk-pixbuf loaders"
HOST_GDK_PIXBUF_ROOT="/usr/lib/${GNU_ARCH}-linux-gnu/gdk-pixbuf-2.0"
GDK_PIXBUF_VERSION_DIR=""
if [ -d "$HOST_GDK_PIXBUF_ROOT" ]; then
  GDK_PIXBUF_VERSION_DIR="$(ls -d "$HOST_GDK_PIXBUF_ROOT"/2.* 2>/dev/null | sort | tail -n1 || true)"
fi
if [ -n "$GDK_PIXBUF_VERSION_DIR" ] && [ -d "${GDK_PIXBUF_VERSION_DIR}/loaders" ]; then
  APPDIR_GDK_PIXBUF="${APPDIR}/usr/lib/gdk-pixbuf-2.0/$(basename "$GDK_PIXBUF_VERSION_DIR")"
  mkdir -p "${APPDIR_GDK_PIXBUF}/loaders"
  cp -L "${GDK_PIXBUF_VERSION_DIR}/loaders/"*.so "${APPDIR_GDK_PIXBUF}/loaders/"
  for loader in "${APPDIR_GDK_PIXBUF}/loaders/"*.so; do
    copy_needed_libs "$loader"
  done
  APPDIR_LOADERS_CACHE="${APPDIR_GDK_PIXBUF}/loaders.cache"
  if command -v "${GNU_ARCH}-linux-gnu-gdk-pixbuf-query-loaders" >/dev/null 2>&1; then
    "${GNU_ARCH}-linux-gnu-gdk-pixbuf-query-loaders" "${APPDIR_GDK_PIXBUF}/loaders/"*.so > "$APPDIR_LOADERS_CACHE"
  elif [ "$ARCH" = "$(dpkg --print-architecture)" ] && command -v gdk-pixbuf-query-loaders >/dev/null 2>&1; then
    gdk-pixbuf-query-loaders "${APPDIR_GDK_PIXBUF}/loaders/"*.so > "$APPDIR_LOADERS_CACHE"
  elif [ -f "${GDK_PIXBUF_VERSION_DIR}/loaders.cache" ]; then
    cp "${GDK_PIXBUF_VERSION_DIR}/loaders.cache" "$APPDIR_LOADERS_CACHE"
  fi
  if [ -f "$APPDIR_LOADERS_CACHE" ]; then
    sed -i "s|\"${APPDIR_GDK_PIXBUF}/|\"|g; s|\"${GDK_PIXBUF_VERSION_DIR}/|\"|g" "$APPDIR_LOADERS_CACHE"
  fi
fi

echo "[4/6] Installing application assets"
cp "${PROJECT_ROOT}/assets/nozik.desktop" "${APPDIR}/usr/share/applications/"
cp -R "${PROJECT_ROOT}/build/icons/hicolor/." "${APPDIR}/usr/share/icons/hicolor/"
ln -sf usr/share/applications/nozik.desktop "${APPDIR}/nozik.desktop"
ln -sf usr/share/icons/hicolor/256x256/apps/nozik.png "${APPDIR}/.DirIcon"
ln -sf usr/share/icons/hicolor/256x256/apps/nozik.png "${APPDIR}/nozik.png"
sed -i 's|^Exec=.*|Exec=nozik|' "${APPDIR}/usr/share/applications/nozik.desktop"
mkdir -p "${APPDIR}/usr/share/nozik"
cp -R "${PROJECT_ROOT}/assets/." "${APPDIR}/usr/share/nozik/"

echo "[5/6] Writing AppRun"
cat > "${APPDIR}/AppRun" <<'EOF'
#!/bin/bash
HERE="$(dirname "$(readlink -f "${0}")")"
export LD_LIBRARY_PATH="${HERE}/usr/lib:${LD_LIBRARY_PATH:-}"
export PATH="${HERE}/usr/bin:${PATH:-}"
export NOZIK_ASSETS_DIR="${HERE}/usr/share/nozik"
if [ -d "${HERE}/usr/lib/gdk-pixbuf-2.0" ]; then
  export GDK_PIXBUF_MODULEDIR="$(ls -d "${HERE}/usr/lib/gdk-pixbuf-2.0"/2.* | sort | tail -n1)/loaders"
  BUNDLED_LOADERS_CACHE="$(dirname "${GDK_PIXBUF_MODULEDIR}")/loaders.cache"
  if [ -f "${BUNDLED_LOADERS_CACHE}" ]; then
    ABS_LOADERS_CACHE="$(mktemp /tmp/nozik-gdk-pixbuf-loaders.XXXXXX.cache)"
    sed "s|\"loaders/|\"${GDK_PIXBUF_MODULEDIR}/|g" "${BUNDLED_LOADERS_CACHE}" > "${ABS_LOADERS_CACHE}"
    export GDK_PIXBUF_MODULE_FILE="${ABS_LOADERS_CACHE}"
  fi
fi
exec "${HERE}/usr/bin/nozik" "$@"
EOF
chmod +x "${APPDIR}/AppRun"

SQUASHFS="${BUILD_DIR}/nozik.squashfs"
rm -f "$SQUASHFS"
mksquashfs "$APPDIR" "$SQUASHFS" -root-owned -noappend -comp xz

RUNTIME_FILE="${BUILD_DIR}/runtime-${RUNTIME_ARCH}"
if [ ! -f "$RUNTIME_FILE" ]; then
  if [ "$ARCH" = riscv64 ]; then
    RUNTIME_URL="https://github.com/redpanda-cpp/appimage-riscv64/releases/download/20230808.0/runtime-riscv64"
  else
    RUNTIME_URL="https://github.com/AppImage/AppImageKit/releases/download/continuous/runtime-${RUNTIME_ARCH}"
  fi
  curl -fL -o "$RUNTIME_FILE" "$RUNTIME_URL"
fi

echo "[6/6] Writing dist/nozik-${ARCH}.AppImage"
cat "$RUNTIME_FILE" "$SQUASHFS" > "${DIST_DIR}/nozik-${ARCH}.AppImage"
chmod +x "${DIST_DIR}/nozik-${ARCH}.AppImage"
