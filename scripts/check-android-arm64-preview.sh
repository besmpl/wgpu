#!/usr/bin/env bash

set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)
: "${GOFFI_DIR:?set GOFFI_DIR to the checked-out goffi Android candidate}"
goffi_dir=$(cd "$GOFFI_DIR" && pwd -P)
actual_head=$(git -C "$goffi_dir" rev-parse HEAD)

if [[ -n "${GOFFI_EXPECTED_HEAD:-}" ]]; then
	if [[ "$actual_head" != "$GOFFI_EXPECTED_HEAD" ]]; then
		echo "goffi HEAD is $actual_head, want $GOFFI_EXPECTED_HEAD" >&2
		exit 1
	fi
fi

: "${ANDROID_NDK_HOME:?set ANDROID_NDK_HOME to Android NDK r29}"
if ! grep -q '^Pkg.Revision = 29\.' "$ANDROID_NDK_HOME/source.properties"; then
	echo "Android NDK r29 is required" >&2
	exit 1
fi

readelf_path=$(find "$ANDROID_NDK_HOME/toolchains/llvm/prebuilt" -path '*/bin/llvm-readelf' \( -type f -o -type l \) -print -quit)
clang_path=$(find "$ANDROID_NDK_HOME/toolchains/llvm/prebuilt" -path '*/bin/aarch64-linux-android29-clang' \( -type f -o -type l \) -print -quit)
if [[ -z "$readelf_path" || -z "$clang_path" ]]; then
	echo "NDK r29 llvm-readelf or API 29 arm64 clang was not found" >&2
	exit 1
fi
"$clang_path" -std=c11 -fsyntax-only "$root/scripts/testdata/android_arm64_vulkan_abi.c"

tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT
(
	cd "$tmpdir"
	GOWORK=off go work init "$root" "$goffi_dir"
)
workspace="$tmpdir/go.work"

selected_goffi=$(GOWORK="$workspace" go list -m -f '{{.Dir}}' github.com/go-webgpu/goffi)
if [[ "$(cd "$selected_goffi" && pwd -P)" != "$goffi_dir" ]]; then
	echo "workspace selected goffi from $selected_goffi, want $goffi_dir" >&2
	exit 1
fi

audit_elf() {
	local binary=$1
	local label=$2
	local dynamic="$tmpdir/$label.dynamic"
	local strings_file="$tmpdir/$label.strings"

	"$readelf_path" -d "$binary" >"$dynamic"
	strings -a "$binary" >"$strings_file"

	grep -Eq 'Shared library: \[libc\.so\]' "$dynamic"
	grep -Eq 'Shared library: \[libdl\.so\]' "$dynamic"
	grep -q 'libvulkan.so' "$strings_file"

	if grep -Eq 'Shared library: \[[^]]*\.so\.[0-9]|Shared library: \[libpthread' "$dynamic"; then
		echo "$label contains a glibc-style or standalone pthread dependency" >&2
		cat "$dynamic" >&2
		exit 1
	fi
	if grep -Eq 'GLIBC_|__errno_location|libX11|libwayland-client|libEGL|libGLESv2' "$strings_file"; then
		echo "$label contains a forbidden non-Bionic/desktop symbol or library" >&2
		exit 1
	fi
}

run_mode() {
	local cgo=$1
	local label="android-arm64-cgo$cgo"
	local binary="$tmpdir/$label"
	local -a env_args=(
		"GOWORK=$workspace"
		"GOOS=android"
		"GOARCH=arm64"
		"CGO_ENABLED=$cgo"
	)
	if [[ "$cgo" == 1 ]]; then
		env_args+=("CC=$clang_path")
	fi

	(
		cd "$root"
		env "${env_args[@]}" go test -exec=true ./...
		env "${env_args[@]}" go build -o "$binary" ./examples/triangle-headless
	)
	audit_elf "$binary" "$label"
}

run_mode 0
run_mode 1

git -C "$root" diff --exit-code -- go.mod go.sum
echo "Android arm64 preview checks passed with goffi $actual_head"
