%global _missing_build_ids_terminate_build 0

Name:           cargadgetd-plugin-maps
Version:        0.0.1
Release:        1
Summary:        Cargadgetd drive map
Group:          Applications/System
License:        MIT
Source0:        %{name}-%{version}.tar.bz2
BuildRequires:  golang
BuildRequires:  gcc
BuildRequires:  gcc-c++
BuildRequires:  make
BuildRequires:  cmake
BuildRequires:  git-core
BuildRequires:  pkgconfig(egl)
BuildRequires:  pkgconfig(glesv2)
BuildRequires:  libpng-devel
BuildRequires:  libjpeg-turbo-devel
BuildRequires:  libicu-devel
BuildRequires:  sqlite-devel
BuildRequires:  libcurl-devel

%description
OSM Scout drive map.


%prep
%setup -q


%build
set -eu
src=$(pwd)
export PATH="/usr/local/cargo/bin:/usr/local/bin:/usr/local/go/bin:${PATH}"
export RUSTUP_HOME=/usr/local/rustup
export CARGO_HOME=/usr/local/cargo
export GOTOOLCHAIN=local
export GO111MODULE=on
export GOPROXY=off
export CGO_ENABLED=1
# Static Sailfish EGL MapLibre. Rust is in the SDK image. Crates, zlib,
# libuv, and the ffi submodule are already in the source tree.
sh third-party/build-sailfish.sh "$src"
export PKG_CONFIG_PATH="$src/_out/maplibre/share/pkgconfig${PKG_CONFIG_PATH:+:$PKG_CONFIG_PATH}"
mkdir -p _out
go build -mod=vendor -buildvcs=false -tags maplibrenative -buildmode=c-shared \
    -ldflags "-s -w" \
    -o _out/libcargadgetd_app_maps.so .


%install
set -eu
rm -rf %{buildroot}
install -d %{buildroot}/usr/lib/cargadgetd/plugins
install -m 755 _out/libcargadgetd_app_maps.so %{buildroot}/usr/lib/cargadgetd/plugins/libcargadgetd_app_maps.so


%files
%defattr(-,root,root,-)
%dir /usr/lib/cargadgetd
%dir /usr/lib/cargadgetd/plugins
/usr/lib/cargadgetd/plugins/libcargadgetd_app_maps.so
