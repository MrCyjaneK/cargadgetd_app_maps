#!/bin/sh
# Real Rust for the maps plugin. The rust/cargo RPMs are scratchbox stubs,
# and /root is hidden by the sfosbuild mount, so the toolchain lives here.
set -eu

export RUSTUP_HOME=/usr/local/rustup
export CARGO_HOME=/usr/local/cargo
export RUSTUP_INIT_SKIP_PATH_CHECK=yes
export PATH="/usr/local/cargo/bin:/usr/local/bin:${PATH}"

# 32-bit rustc links the host tools with -latomic. The SDK targets do not
# install that RPM. aarch64 rustup is not linked to it.
case $(uname -m) in
i486|i686|armv7l|armv7hl)
	zypper --non-interactive install --no-recommends libatomic
	;;
esac

if ! /usr/local/cargo/bin/rustc --version >/dev/null 2>&1; then
	curl --proto '=https' --tlsv1.2 -fsSL https://sh.rustup.rs | \
		sh -s -- -y --default-toolchain stable --profile minimal --no-modify-path
fi
rustc --version

if ! command -v cargo-about >/dev/null 2>&1; then
	cargo install cargo-about --locked --features cli --root /usr/local
fi
cargo-about --version

# The rust and cargo RPMs are scratchbox stubs earlier on PATH.
ln -sfn /usr/local/cargo/bin/rustc /usr/bin/rustc
ln -sfn /usr/local/cargo/bin/cargo /usr/bin/cargo
ln -sfn /usr/local/bin/cargo-about /usr/bin/cargo-about

cat > /etc/profile.d/cargadgetd-rust.sh << 'EOF'
export RUSTUP_HOME=/usr/local/rustup
export CARGO_HOME=/usr/local/cargo
export PATH="/usr/local/cargo/bin:/usr/local/bin:${PATH}"
EOF
