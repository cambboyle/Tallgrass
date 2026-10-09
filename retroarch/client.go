// Package retroarch talks to RetroArch's UDP network command interface.
package retroarch

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// ParseReply turns a READ_CORE_MEMORY text reply into raw bytes.
func ParseReply(reply string) ([]byte, error) {
	words := strings.Fields(reply)

	if len(words) < 3 {
		return nil, fmt.Errorf("unexpected reply: %q", reply)
	}

	if words[2] == "-1" {
		return nil, fmt.Errorf("RetroArch refused the read: %s", strings.Join(words[3:], " "))
	}

	hexWords := words[2:]
	data := make([]byte, len(hexWords))

	for i, word := range hexWords {
		value, err := strconv.ParseUint(word, 16, 8)
		if err != nil {
			return nil, fmt.Errorf("byte %d (%q) isn't valid hex: %w", i, word, err)
		}
		data[i] = byte(value)
	}

	return data, nil
}

// MaxChunk is the most bytes asked for in one command. Replies are text
// (3 chars per byte) in a single UDP datagram, which can't exceed 64 KB.
// A variable so tests can shrink it.
var MaxChunk = 8192

// ReadMemory reads length bytes of core memory starting at address,
// splitting large reads into several commands.
func ReadMemory(conn net.Conn, address uint32, length int) ([]byte, error) {
	data := make([]byte, 0, length)
	for len(data) < length {
		n := min(MaxChunk, length-len(data))
		chunk, err := readChunk(conn, address+uint32(len(data)), n)
		if err != nil {
			return nil, err
		}
		data = append(data, chunk...)
	}
	return data, nil
}

func readChunk(conn net.Conn, address uint32, length int) ([]byte, error) {
	cmd := fmt.Sprintf("READ_CORE_MEMORY %08X %d", address, length)

	_, err := conn.Write([]byte(cmd))
	if err != nil {
		return nil, err
	}

	// A reply that arrived after an earlier read timed out can still be
	// queued, so skip anything that isn't for this address.
	want := fmt.Sprintf("read_core_memory %x ", address) // compared lowercased; RetroArch replies in lowercase hex
	conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	// Each byte comes back as "XX " (3 chars). An undersized buffer would
	// silently truncate the UDP datagram.
	buf := make([]byte, len(want)+3*length+64)
	var reply string
	for !strings.HasPrefix(strings.ToLower(reply), want) {
		n, err := conn.Read(buf)
		if err != nil {
			return nil, err
		}
		reply = string(buf[:n])
	}

	data, err := ParseReply(reply)
	if err != nil {
		return nil, err
	}

	if len(data) != length {
		return nil, fmt.Errorf("asked for %d bytes, got %d", length, len(data))
	}

	return data, nil
}
