package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/term"

	"tallgrass/savesync"
)

// syncMain runs "tallgrass sync": keep a folder on this computer in step
// with a device's saves, for desktop save editors such as OpenHome.
func syncMain(args []string) {
	fs := flag.NewFlagSet("sync", flag.ExitOnError)
	home, _ := os.UserHomeDir()
	dir := fs.String("dir", filepath.Join(home, "Tallgrass Saves"), "folder to keep the saves in")
	interval := fs.Duration("interval", 5*time.Second, "how often to check for changes")
	once := fs.Bool("once", false, "sync once and exit")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: tallgrass sync [flags] <device address, e.g. 192.168.0.227>")
		fmt.Fprintln(os.Stderr, "Keeps a folder here in step with the device's saves: open them in OpenHome or PKHeX, and saved edits are uploaded back.")
		fs.PrintDefaults()
	}
	fs.Parse(args)
	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(2)
	}
	s, err := savesync.New(deviceURL(fs.Arg(0)), *dir)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("syncing %s with %s", s.Device, s.Dir)
	for {
		if *once {
			err = s.Once()
		} else {
			err = s.Run(*interval)
		}
		if !errors.Is(err, savesync.ErrPINNeeded) {
			break
		}
		pin, perr := readPIN()
		if perr != nil {
			log.Fatal(perr)
		}
		if err := s.SignIn(pin); err != nil {
			log.Fatal(err)
		}
	}
	if err != nil {
		log.Fatal(err)
	}
}

// deviceURL accepts "192.168.0.227", "retropie:8080" or a full URL.
func deviceURL(s string) string {
	if !strings.Contains(s, "://") {
		s = "http://" + s
	}
	if host := strings.SplitN(strings.SplitN(s, "://", 2)[1], "/", 2)[0]; !strings.Contains(host, ":") {
		s = strings.Replace(s, host, host+":8080", 1)
	}
	return strings.TrimRight(s, "/")
}

func readPIN() (string, error) {
	fmt.Fprint(os.Stderr, "Dashboard PIN: ")
	if term.IsTerminal(int(os.Stdin.Fd())) {
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		return string(b), err
	}
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	return strings.TrimSpace(line), err
}
