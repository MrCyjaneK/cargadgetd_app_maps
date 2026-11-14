#!/bin/sh
# Sailfish EGL build of the maplibre-native-ffi submodule.
# The RPM tree has no network. Rust crates are in third-party/cargo-vendor.
# zlib and libuv are sibling submodules. maplibre-native is a submodule of
# the ffi repo; its upstream commit is what gets packed, so the ffi patches
# are applied here.
set -eu
src=${1:?repo root}
ffi=$src/third-party/maplibre-native-ffi
zlib=$src/third-party/zlib
uv=$src/third-party/libuv
vendor=$src/third-party/cargo-vendor
mln=$ffi/third_party/maplibre-native
# rpmbuild mounts the workspace read-only and builds in a copy. A cmake
# cache left in the tree points at that read-only path, so the objects
# live in the container instead.
build=/var/tmp/cargadgetd-maplibre
prefix=$src/_out/maplibre

export RUSTUP_HOME=/usr/local/rustup
export CARGO_HOME=/usr/local/cargo
export PATH="/usr/local/cargo/bin:/usr/local/bin:${PATH}"
if ! command -v cargo >/dev/null 2>&1 || cargo --version 2>&1 | grep -q stub; then
	echo "maps: need the SDK image rustup at /usr/local/cargo (the /usr/bin/cargo RPM is a scratchbox stub)" >&2
	exit 1
fi
command -v cargo-about >/dev/null 2>&1 || {
	echo "maps: cargo-about is not on PATH (the SDK image installs it to /usr/local/bin)" >&2
	exit 1
}
for need in "$ffi/CMakeLists.txt" "$zlib/CMakeLists.txt" "$uv/CMakeLists.txt" "$vendor" "$mln/CMakeLists.txt"; do
	if [ ! -e "$need" ]; then
		echo "maps: missing $need" >&2
		exit 1
	fi
done

if [ -d "$ffi/patches/maplibre-native" ]; then
	for patch in "$ffi/patches/maplibre-native"/*.patch; do
		[ -f "$patch" ] || continue
		echo "maps: apply $(basename "$patch")"
		# The packed tree is not a git checkout. git apply still applies
		# these patches, including the binary image hunks patch(1) rejects.
		(cd "$mln" && git apply --unsafe-paths "$patch")
	done
fi

mkdir -p "$ffi/.cargo"
cat > "$ffi/.cargo/config.toml" << EOF
[source.crates-io]
replace-with = "vendored"

[source.vendored]
directory = "$vendor"
EOF
export CARGO_NET_OFFLINE=true

# Sailfish GCC rejects clang's -fno-c++-static-destructors. The sources do
# not register static destructors; the flag is only a clang safety check.
python3 - "$ffi/cmake/mln_ffi_c_api.cmake" << 'PY'
import pathlib, sys
p = pathlib.Path(sys.argv[1])
t = p.read_text()
needle = 'function(mln_ffi_no_static_destructors_option out_var)\n  if(CMAKE_CXX_COMPILER_FRONTEND_VARIANT STREQUAL "MSVC")'
insert = '''function(mln_ffi_no_static_destructors_option out_var)
  if(CMAKE_CXX_COMPILER_ID STREQUAL "GNU")
    set(${out_var} "" PARENT_SCOPE)
    return()
  endif()
  if(CMAKE_CXX_COMPILER_FRONTEND_VARIANT STREQUAL "MSVC")'''
if 'CMAKE_CXX_COMPILER_ID STREQUAL "GNU"' not in t:
    if needle not in t:
        sys.exit('maps: maplibre cmake no longer matches the Sailfish GCC fix')
    p.write_text(t.replace(needle, insert, 1))
PY

# The ffi platform check allows x86_64 and aarch64 only. Sailfish i486
# reports i686. armv7hl reports armv7l, and its rustup triple is
# armv7-unknown-linux-gnueabihf.
python3 - "$ffi/cmake/mln_ffi_platform.cmake" << 'PY'
import pathlib, sys
p = pathlib.Path(sys.argv[1])
t = p.read_text()
old = '''  elseif(CMAKE_SYSTEM_PROCESSOR MATCHES "^(aarch64|arm64)$")
    set(MLN_FFI_DETECTED_ARCHITECTURE arm64)'''
new = '''  elseif(CMAKE_SYSTEM_PROCESSOR MATCHES "^(aarch64|arm64)$")
    set(MLN_FFI_DETECTED_ARCHITECTURE arm64)
  elseif(CMAKE_SYSTEM_PROCESSOR MATCHES "^(i486|i686|i386)$")
    set(MLN_FFI_DETECTED_ARCHITECTURE i686)
  elseif(CMAKE_SYSTEM_PROCESSOR MATCHES "^(armv7l|armv7hl|armv7)$")
    set(MLN_FFI_DETECTED_ARCHITECTURE armv7)'''
old_rust = '''  elseif(MLN_FFI_RUST_TARGET MATCHES "^aarch64-")
    set(MLN_FFI_RUST_ARCHITECTURE arm64)'''
new_rust = '''  elseif(MLN_FFI_RUST_TARGET MATCHES "^aarch64-")
    set(MLN_FFI_RUST_ARCHITECTURE arm64)
  elseif(MLN_FFI_RUST_TARGET MATCHES "^i686-")
    set(MLN_FFI_RUST_ARCHITECTURE i686)
  elseif(MLN_FFI_RUST_TARGET MATCHES "^armv7-")
    set(MLN_FFI_RUST_ARCHITECTURE armv7)'''
if 'DETECTED_ARCHITECTURE i686' not in t:
    if old not in t:
        sys.exit('maps: maplibre platform check no longer matches the 32-bit fix')
    t = t.replace(old, new, 1)
elif 'DETECTED_ARCHITECTURE armv7' not in t:
    sys.exit('maps: maplibre platform check has i686 but not armv7')
if 'RUST_ARCHITECTURE i686' not in t:
    if old_rust not in t:
        sys.exit('maps: maplibre rust arch check no longer matches the 32-bit fix')
    t = t.replace(old_rust, new_rust, 1)
elif 'RUST_ARCHITECTURE armv7' not in t:
    sys.exit('maps: maplibre rust arch check has i686 but not armv7')
p.write_text(t)
PY

case $(uname -m) in
aarch64|arm64)
	mln_arch=arm64
	mln_platform=linux-gnu-arm64
	mln_zig=aarch64-linux-gnu
	mln_rust=aarch64-unknown-linux-gnu
	mln_artifact=sailfish-aarch64-egl
	;;
i486|i686)
	mln_arch=i686
	mln_platform=linux-gnu-i686
	mln_zig=i686-linux-gnu
	mln_rust=i686-unknown-linux-gnu
	mln_artifact=sailfish-i686-egl
	;;
armv7l|armv7hl)
	mln_arch=armv7
	mln_platform=linux-gnu-armv7
	mln_zig=armv7-linux-gnueabihf
	mln_rust=armv7-unknown-linux-gnueabihf
	mln_artifact=sailfish-armv7-egl
	;;
*)
	echo "maps: unsupported architecture $(uname -m)" >&2
	exit 1
	;;
esac

cmake -S "$ffi" -B "$build" \
	-DCMAKE_BUILD_TYPE=Release \
	-DCMAKE_INSTALL_PREFIX="$prefix" \
	-DCMAKE_INSTALL_LIBDIR=lib \
	-DMLN_FFI_RENDER_BACKEND=opengl \
	-DMLN_FFI_OPENGL_CONTEXT_PROVIDER=egl \
	-DMLN_FFI_TARGET_ARCHITECTURE="$mln_arch" \
	-DMLN_FFI_TARGET_PLATFORM="$mln_platform" \
	-DMLN_FFI_ZIG_TARGET="$mln_zig" \
	-DMLN_FFI_RUST_TARGET="$mln_rust" \
	-DMLN_FFI_DEFAULT_RENDER_BACKEND=opengl \
	-DMLN_FFI_ARTIFACT_NAME="$mln_artifact" \
	-DBUILD_TESTING=OFF \
	-DFETCHCONTENT_SOURCE_DIR_MLN_FFI_ZLIB_SOURCE="$zlib" \
	-DFETCHCONTENT_SOURCE_DIR_MLN_FFI_LIBUV_SOURCE="$uv" \
	-DFETCHCONTENT_FULLY_DISCONNECTED=ON
cmake --build "$build" -j"$(nproc)"
cmake --install "$build"
rm -f "$prefix"/lib/libmaplibre-native-c.so "$prefix"/lib/libmaplibre-native-c.so.*

mkdir -p "$prefix/share/pkgconfig"
cat > "$prefix/share/pkgconfig/maplibre-native-c.pc" << EOF
prefix=$prefix
libdir=\${prefix}/lib
includedir=\${prefix}/include
Name: maplibre-native-c
Description: MapLibre Native C API, static, Sailfish EGL
Version: 0.1.0
Cflags: -I\${includedir} -std=c2x
Libs: -L\${libdir} -lmaplibre-native-c -lmln_ffi_platform -lstdc++ -lgcc_s -ldl -lm -pthread
EOF
