package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"tallgrass/cloud"
	"tallgrass/dashboard"
	"tallgrass/emulator"
	"tallgrass/gen3"
	"tallgrass/profile"
	"tallgrass/retroarch"
	"tallgrass/saves"
	"tallgrass/tracker"
)

// version is set at build time (-ldflags "-X main.version=0.1.0").
var version = "dev"

func main() {
	host := flag.String("host", "127.0.0.1:55355", "RetroArch network command address")
	profilePath := flag.String("profile", "radical-red", "profile to use where the running game can't be detected (not Linux), or a path to a .json profile")
	listen := flag.String("listen", ":8080", "dashboard address")
	interval := flag.Duration("interval", time.Second, "how often to read the party")
	printOnce := flag.Bool("print", false, "print the party to the terminal and exit")
	dbPath := flag.String("db", "tallgrass.db", "SQLite file for the Nuzlocke log (empty to disable)")
	romsDir := flag.String("roms", "/home/pi/RetroPie/roms", "RetroPie ROMs folder whose save files get backed up")
	backupDir := flag.String("backups", "backups", "where save file versions are kept")
	drasticSaves := flag.String("drastic-saves", "/opt/retropie/configs/nds/drastic/backup", "DraStic's save folder (DS games)")
	retropieConfigs := flag.String("retropie-configs", "/opt/retropie/configs", "RetroPie's config folder, for the setup wizard")
	profilesDir := flag.String("profiles-dir", "profiles", "folder of extra game profiles (*.json); they take priority over the built-in ones")
	esGamelists := flag.String("es-gamelists", defaultESGamelists(), "EmulationStation's gamelists folder, for naming games in its menus")
	drasticDir := flag.String("drastic-dir", "/opt/retropie/emulators/drastic", "where DraStic is installed, for the setup wizard")
	authFile := flag.String("auth-file", "auth.json", "where the dashboard PIN (hashed) and sign-ins are kept; delete it to remove a forgotten PIN")
	cloudURL := flag.String("cloud", cloud.DefaultURL, "Tallgrass backend (empty to turn accounts off)")
	siteURL := flag.String("site", "https://tallgrass-delta.vercel.app", "Tallgrass website, where pairing codes are entered")
	linkPath := flag.String("link-file", "cloud-link.json", "where this device's account key is kept")
	showVersion := flag.Bool("version", false, "print the version and exit")
	noUpdates := flag.Bool("no-updates", false, "don't check GitHub for new releases")
	exportROM := flag.String("export", "", "write a ROM's game data (species, moves, trainers) as JSON to stdout and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println("tallgrass", version)
		return
	}

	if *exportROM != "" {
		bundled, local, err := loadProfiles(*profilesDir)
		if err != nil {
			log.Fatal(err)
		}
		all := append(local, bundled...)
		gd, err := dashboard.New(all[0], nil).ExportFile(*exportROM, all)
		if err != nil {
			log.Fatal(err)
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", " ")
		if err := enc.Encode(gd); err != nil {
			log.Fatal(err)
		}
		return
	}

	prof, err := profile.Load(*profilePath)
	if err != nil {
		log.Fatal(err)
	}

	conn, err := net.Dial("udp", *host)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	// One request at a time: the poller and web requests share the socket,
	// and a reply must reach the read that asked for it.
	var memMu sync.Mutex
	readMemory := func(addr uint32, n int) ([]byte, error) {
		memMu.Lock()
		defer memMu.Unlock()
		return retroarch.ReadMemory(conn, addr, n)
	}

	dash := dashboard.New(prof, readMemory)

	if *printOnce {
		printParty(dash.Snapshot())
		return
	}

	if *dbPath != "" {
		store, err := tracker.Open(*dbPath)
		if err != nil {
			log.Fatal(err)
		}
		defer store.Close()
		dash.EnableTracking(store)
		log.Printf("logging events to %s", *dbPath)
	}

	bundled, local, err := loadProfiles(*profilesDir)
	if err != nil {
		log.Fatal(err)
	}
	all := append(append([]*profile.Profile{}, local...), bundled...)
	if emulator.Supported("/proc") {
		dash.EnableDetection(func() (*emulator.Running, error) { return emulator.Detect("/proc") }, all)
		log.Printf("following whichever game is running")
	} else {
		log.Printf("can't see processes here; using the %s profile", prof.Name)
	}

	if _, err := os.Stat(*romsDir); err == nil {
		watcher := saves.NewWatcher(saves.RetroPieSources(*romsDir, *drasticSaves), saves.NewBackups(*backupDir))
		go watcher.Run(10 * time.Second)
		dash.EnableSaves(watcher, all)
		dash.EnableHashCache("rom-hashes.json")
		log.Printf("backing up saves from %s to %s", *romsDir, *backupDir)
	} else {
		log.Printf("save backups off: %v", err)
	}

	if *cloudURL != "" {
		// New and fixed profiles from the registry, without a new release.
		dash.EnableRegistry(cloud.New(*cloudURL, cloud.DefaultAnonKey), "registry-profiles.json", version, local, bundled)
	}

	dash.EnableSetup(*retropieConfigs, *drasticDir)
	dash.EnableGamelists(*esGamelists)
	if !*noUpdates {
		dash.EnableUpdates(version)
	}
	if err := dash.EnableAuth(*authFile); err != nil {
		log.Fatalf("reading %s: %v", *authFile, err)
	}

	if *cloudURL != "" {
		dash.EnableCloud(cloud.New(*cloudURL, cloud.DefaultAnonKey), *linkPath, *siteURL)
		dash.EnableCloudSync("cloud-sync.json")
	}

	go dash.Poll(*interval)

	addr := *listen
	if strings.HasPrefix(addr, ":") {
		addr = "localhost" + addr
	}
	log.Printf("dashboard on http://%s (RetroArch at %s)", addr, *host)
	log.Fatal(http.ListenAndServe(*listen, dash.Handler()))
}

func printParty(st dashboard.State) {
	if !st.Connected {
		log.Fatal(st.Error)
	}

	fmt.Printf("%s party\n\n", st.Game)
	for i, m := range st.Party {
		if m == nil {
			fmt.Printf("Slot %d: (empty)\n", i+1)
			continue
		}
		printMember(i+1, m)
	}
}

func printMember(slot int, m *dashboard.Member) {
	egg := ""
	if m.IsEgg {
		egg = " [egg]"
	}
	fmt.Printf("Slot %d: %s (%s, #%d) Lv %d%s\n", slot, m.Nickname, m.SpeciesName, m.Species, m.Level, egg)
	fmt.Printf("  OT %s  Nature %s  Ability slot %d  Item %s  Friendship %d  Exp %d\n",
		m.OTName, m.Nature, m.AbilitySlot, m.ItemName, m.Friendship, m.Experience)
	fmt.Printf("  HP %d/%d\n", m.CurrentHP, m.Stats.HP)
	fmt.Printf("  %-6s %4s %4s %4s %4s %4s %4s\n", "", "HP", "Atk", "Def", "SpA", "SpD", "Spe")
	printStats("Stats", m.Stats)
	printStats("IVs", m.IVs)
	printStats("EVs", m.EVs)
	fmt.Print("  Moves:")
	for i, mv := range m.Moves {
		fmt.Printf("  %s (%d PP)", m.MoveNames[i], mv.PP)
	}
	fmt.Println()
	fmt.Println()
}

func printStats(label string, s gen3.Stats) {
	fmt.Printf("  %-6s %4d %4d %4d %4d %4d %4d\n", label, s.HP, s.Attack, s.Defense, s.SpAtk, s.SpDef, s.Speed)
}

// defaultESGamelists is where RetroPie's EmulationStation keeps its game
// lists: ~/.emulationstation/gamelists.
func defaultESGamelists() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home + "/.emulationstation/gamelists"
}

// loadProfiles returns the bundled profiles and any in dir (which win).
func loadProfiles(dir string) (bundled, local []*profile.Profile, err error) {
	bundled, err = profile.All()
	if err != nil {
		return nil, nil, err
	}
	all, errs := profile.WithLocal(nil, dir)
	for _, err := range errs {
		log.Printf("skipping a local profile: %v", err)
	}
	return bundled, all, nil
}
