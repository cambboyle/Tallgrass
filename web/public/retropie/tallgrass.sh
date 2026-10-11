#!/usr/bin/env bash

# RetroPie-Setup module for Tallgrass, a Pokémon companion dashboard
# (https://github.com/cambboyle/Tallgrass, GPL-3.0).
#
# Add it to RetroPie-Setup:
#
#   mkdir -p ~/RetroPie-Setup/ext/tallgrass/scriptmodules/supplementary
#   curl -fsSLo ~/RetroPie-Setup/ext/tallgrass/scriptmodules/supplementary/tallgrass.sh \
#     https://tallgrass-delta.vercel.app/retropie/tallgrass.sh
#
# then RetroPie-Setup → Manage packages → Manage experimental packages →
# tallgrass. It runs the release installer (install.sh) for the RetroPie
# user, so the result is the same as the one-line install: ~/tallgrass, a
# systemd service, and updates from the dashboard.

rp_module_id="tallgrass"
rp_module_desc="Tallgrass - Pokémon companion: live party, route encounters, Nuzlocke log and save backups in a web dashboard"
rp_module_help="After installing, open http://<this device's IP>:8080/setup.html on your phone or computer.\n\nTallgrass keeps itself up to date (dashboard: Account → Updates). Its data (save backups, the Journey, settings) lives in ~/tallgrass and is kept if you remove the package."
rp_module_licence="GPL3 https://raw.githubusercontent.com/cambboyle/Tallgrass/main/LICENSE"
rp_module_section="exp"
rp_module_flags="noinstclean"

function depends_tallgrass() {
    getDepends curl
}

function install_bin_tallgrass() {
    local tmp="$(mktemp -d)"
    download "https://github.com/cambboyle/Tallgrass/releases/latest/download/install.sh" "$tmp/install.sh" || { rm -rf "$tmp"; return 1; }
    TALLGRASS_USER="$user" bash "$tmp/install.sh" || md_ret_errors+=("The Tallgrass installer failed")
    rm -rf "$tmp"
    # RetroPie-Setup counts a package as installed while this folder exists.
    mkdir -p "$md_inst"
    ln -sf "$home/tallgrass/tallgrass" "$md_inst/tallgrass"
}

function remove_tallgrass() {
    systemctl disable --now tallgrass 2>/dev/null
    rm -f /etc/systemd/system/tallgrass.service
    systemctl daemon-reload
    # Only the program: save backups and the Journey stay in ~/tallgrass.
    rm -f "$home/tallgrass/tallgrass"
}

function gui_tallgrass() {
    local ip="$(getIPAddress)"
    local cmd=(dialog --backtitle "$__backtitle" --menu "Tallgrass $("$home/tallgrass/tallgrass" -version 2>/dev/null | cut -d' ' -f2)" 22 76 16)
    local options=(
        1 "Show the dashboard address"
        2 "Restart Tallgrass"
    )
    local choice
    while true; do
        choice=$("${cmd[@]}" "${options[@]}" 2>&1 >/dev/tty)
        case "$choice" in
            1)
                printMsgs "dialog" "Open this on your phone or computer (same network):\n\nhttp://${ip:-this-device}:8080\n\nService: $(systemctl is-active tallgrass)"
                ;;
            2)
                systemctl restart tallgrass
                printMsgs "dialog" "Tallgrass restarted: $(systemctl is-active tallgrass)"
                ;;
            *)
                break
                ;;
        esac
    done
}
