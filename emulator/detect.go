// Package emulator finds which game is running, by reading emulator
// processes' command lines from /proc (Linux only).
package emulator

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Running is a game open in an emulator.
type Running struct {
	Emulator string // "retroarch" or "drastic"
	PID      int
	ROM      string // absolute path
}

// Live reports whether the game's memory can be read through RetroArch's
// network commands. DraStic games are read from its process memory instead
// (see dashboard/dslive.go).
func (r *Running) Live() bool { return r.Emulator == "retroarch" }

var emulators = map[string]bool{"retroarch": true, "drastic": true}

var romExts = map[string]bool{".gba": true, ".gb": true, ".gbc": true, ".nds": true, ".zip": true, ".7z": true}

// Supported reports whether this machine can be scanned (it has /proc).
func Supported(proc string) bool {
	_, err := os.Stat(filepath.Join(proc, "self", "cmdline"))
	return err == nil
}

// Detect returns the running game, or nil if no known emulator has one open.
// proc is normally "/proc".
func Detect(proc string) (*Running, error) {
	entries, err := os.ReadDir(proc)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue // not a process
		}
		raw, err := os.ReadFile(filepath.Join(proc, e.Name(), "cmdline"))
		if err != nil || len(raw) == 0 {
			continue // exited, or a kernel thread
		}
		args := strings.Split(strings.TrimRight(string(raw), "\x00"), "\x00")
		name := filepath.Base(args[0])
		if !emulators[name] {
			continue // includes launcher scripts, whose argv[0] is the shell
		}
		// The ROM is the last argument with a ROM extension; RetroArch also
		// gets core and config paths.
		for i := len(args) - 1; i > 0; i-- {
			if romExts[strings.ToLower(filepath.Ext(args[i]))] {
				return &Running{Emulator: name, PID: pid, ROM: args[i]}, nil
			}
		}
	}
	return nil, nil
}
