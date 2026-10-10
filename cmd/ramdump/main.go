// ramdump saves a running game's memory to files, for working out where
// data lives. GBA: EWRAM and IWRAM through RetroArch. DS: main RAM from
// DraStic's process.
package main

import (
	"fmt"
	"net"
	"os"

	"tallgrass/emulator"
	"tallgrass/gen4"
	"tallgrass/retroarch"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "ds" {
		r, err := emulator.Detect("/proc")
		if err != nil || r == nil {
			fmt.Println("no game running", err)
			os.Exit(1)
		}
		mem, err := emulator.OpenProcMem("/proc", r.PID)
		if err != nil {
			panic(err)
		}
		b, err := mem.Read(gen4.MainRAM, gen4.MainRAMSize)
		if err != nil {
			panic(err)
		}
		os.WriteFile("ds-ram.bin", b, 0o644)
		fmt.Println("wrote ds-ram.bin", len(b))
		return
	}
	conn, err := net.Dial("udp", "127.0.0.1:55355")
	if err != nil {
		panic(err)
	}
	for _, r := range []struct {
		name       string
		addr, size uint32
	}{{"ewram.bin", 0x02000000, 0x40000}, {"iwram.bin", 0x03000000, 0x8000}} {
		var out []byte
		for off := uint32(0); off < r.size; off += 0x2000 {
			b, err := retroarch.ReadMemory(conn, r.addr+off, 0x2000)
			if err != nil {
				panic(fmt.Sprintf("%s at %x: %v", r.name, r.addr+off, err))
			}
			out = append(out, b...)
		}
		os.WriteFile(r.name, out, 0o644)
		fmt.Println("wrote", r.name, len(out))
	}
}
