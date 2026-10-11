#!/usr/bin/env bash
# Install or update Tallgrass on a Raspberry Pi (RetroPie) or Linux PC.
#
#   curl -fsSL https://tallgrass-delta.vercel.app/install.sh | bash
#
# It downloads the latest release for this device, checks it against the
# release's checksums, installs it to ~/tallgrass, and runs it as a service
# (Tallgrass then keeps itself up to date). Options, as environment
# variables: TALLGRASS_VERSION=0.1.0 for a specific release,
# TALLGRASS_DIR=/path to install somewhere else. Run as root (sudo, or the
# RetroPie-Setup module), it installs for TALLGRASS_USER, else the user who
# ran sudo.
set -euo pipefail

REPO="cambboyle/Tallgrass"
VERSION="${TALLGRASS_VERSION:-latest}"
SUDO=sudo
TARGET_USER="$(id -un)"
if [ "$(id -u)" -eq 0 ]; then
  SUDO=""
  TARGET_USER="${TALLGRASS_USER:-${SUDO_USER:-root}}"
fi
TARGET_HOME="$(getent passwd "$TARGET_USER" | cut -d: -f6)"
TARGET_HOME="${TARGET_HOME:-$HOME}"
DIR="${TALLGRASS_DIR:-$TARGET_HOME/tallgrass}"

say() { printf '\033[1;32m==>\033[0m %s\n' "$*"; }
die() { printf '\033[1;31mError:\033[0m %s\n' "$*" >&2; exit 1; }

case "$(uname -m)" in
  armv7l|armv6l) ARCH=armv7 ;;
  aarch64|arm64) ARCH=arm64 ;;
  x86_64|amd64) ARCH=amd64 ;;
  *) die "Tallgrass doesn't have a build for $(uname -m) yet." ;;
esac
[ "$(uname -s)" = "Linux" ] || die "this installer is for Linux (Raspberry Pi, RetroPie, Batocera, Steam Deck desktop)."
command -v curl >/dev/null || die "curl is needed."
command -v sha256sum >/dev/null || die "sha256sum is needed."

if [ "$VERSION" = "latest" ]; then
  BASE="https://github.com/$REPO/releases/latest/download"
else
  BASE="https://github.com/$REPO/releases/download/v${VERSION#v}"
fi
BIN="tallgrass-linux-$ARCH"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
say "Downloading Tallgrass ($VERSION) for $ARCH"
curl -fsSL "$BASE/$BIN" -o "$TMP/$BIN" || die "couldn't download $BASE/$BIN"
curl -fsSL "$BASE/SHA256SUMS" -o "$TMP/SHA256SUMS" || die "couldn't download the checksums"
(cd "$TMP" && grep " $BIN\$" SHA256SUMS | sha256sum -c --quiet -) || die "the download doesn't match the release's checksum"

mkdir -p "$DIR"
mv "$TMP/$BIN" "$DIR/tallgrass.new"
chmod 755 "$DIR/tallgrass.new"
mv "$DIR/tallgrass.new" "$DIR/tallgrass"
# The service runs as TARGET_USER and replaces its own binary when updating.
[ "$(id -un)" = "$TARGET_USER" ] || chown "$TARGET_USER:" "$DIR" "$DIR/tallgrass"
say "Installed $("$DIR/tallgrass" -version) to $DIR"

# Where things are on this system: RetroPie keeps ROMs in ~/RetroPie/roms.
ROMS="$TARGET_HOME/RetroPie/roms"
[ -d "$ROMS" ] || ROMS="$TARGET_HOME/roms"

if command -v systemctl >/dev/null && [ -d /run/systemd/system ]; then
  say "Setting up the tallgrass service${SUDO:+ (needs sudo)}"
  $SUDO tee /etc/systemd/system/tallgrass.service >/dev/null <<UNIT
[Unit]
Description=Tallgrass dashboard
After=network-online.target
Wants=network-online.target

[Service]
User=$TARGET_USER
WorkingDirectory=$DIR
ExecStart=$DIR/tallgrass -host 127.0.0.1:55355 -listen :8080 -roms $ROMS
Restart=always
RestartSec=5
# Give the emulator priority over us for CPU and disk.
Nice=10
IOSchedulingClass=idle

[Install]
WantedBy=multi-user.target
UNIT
  $SUDO systemctl daemon-reload
  $SUDO systemctl enable tallgrass >/dev/null 2>&1
  $SUDO systemctl restart tallgrass
else
  say "No systemd here: start Tallgrass yourself with: cd $DIR && ./tallgrass"
fi

IP="$(hostname -I 2>/dev/null | awk '{print $1}')"
say "Done. Open http://${IP:-this-device}:8080/setup.html on your phone or computer to set up your game."
