#!/usr/bin/env bash
# Install or update Tallgrass on a Raspberry Pi (RetroPie) or Linux PC.
#
#   curl -fsSL https://tallgrass-delta.vercel.app/install.sh | bash
#
# It downloads the latest release for this device, checks it against the
# release's checksums, installs it to ~/tallgrass, and runs it as a service
# (Tallgrass then keeps itself up to date). Options, as environment
# variables: TALLGRASS_VERSION=0.1.0 for a specific release,
# TALLGRASS_DIR=/path to install somewhere else.
set -euo pipefail

REPO="cambboyle/Tallgrass"
DIR="${TALLGRASS_DIR:-$HOME/tallgrass}"
VERSION="${TALLGRASS_VERSION:-latest}"

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
say "Installed $("$DIR/tallgrass" -version) to $DIR"

# Where things are on this system: RetroPie keeps ROMs in ~/RetroPie/roms.
ROMS="$HOME/RetroPie/roms"
[ -d "$ROMS" ] || ROMS="$HOME/roms"

if command -v systemctl >/dev/null && [ -d /run/systemd/system ]; then
  say "Setting up the tallgrass service (needs sudo)"
  sudo tee /etc/systemd/system/tallgrass.service >/dev/null <<UNIT
[Unit]
Description=Tallgrass dashboard
After=network-online.target
Wants=network-online.target

[Service]
User=$USER
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
  sudo systemctl daemon-reload
  sudo systemctl enable tallgrass >/dev/null 2>&1
  sudo systemctl restart tallgrass
else
  say "No systemd here: start Tallgrass yourself with: cd $DIR && ./tallgrass"
fi

IP="$(hostname -I 2>/dev/null | awk '{print $1}')"
say "Done. Open http://${IP:-this-device}:8080/setup.html on your phone or computer to set up your game."
