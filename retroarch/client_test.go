package retroarch

import (
	"fmt"
	"net"
	"sync/atomic"
	"testing"
)

// echoRetroArch answers every read with bytes equal to the low byte of
// each address, like real memory would.
func echoRetroArch(t *testing.T) (net.Conn, *atomic.Int32) {
	t.Helper()
	server, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { server.Close() })
	requests := new(atomic.Int32)
	go func() {
		buf := make([]byte, 256)
		for {
			n, client, err := server.ReadFrom(buf)
			if err != nil {
				return
			}
			requests.Add(1)
			var addr uint32
			var length int
			fmt.Sscanf(string(buf[:n]), "READ_CORE_MEMORY %x %d", &addr, &length)
			reply := fmt.Sprintf("READ_CORE_MEMORY %x", addr)
			for i := 0; i < length; i++ {
				reply += fmt.Sprintf(" %02X", byte(addr+uint32(i)))
			}
			server.WriteTo([]byte(reply), client)
		}
	}()
	conn, err := net.Dial("udp", server.LocalAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn, requests
}

func TestReadMemoryChunks(t *testing.T) {
	// The fake can't send 8 KB chunks: macOS caps UDP datagrams at 9216 bytes.
	defer func(n int) { MaxChunk = n }(MaxChunk)
	MaxChunk = 1000
	conn, requests := echoRetroArch(t)
	b, err := ReadMemory(conn, 0x02029314, 2*MaxChunk+100)
	if err != nil {
		t.Fatal(err)
	}
	if n := requests.Load(); n != 3 {
		t.Errorf("%d requests, want 3", n)
	}
	for i, v := range b {
		if v != byte(0x14+i) {
			t.Fatalf("byte %d = %02X", i, v)
		}
	}
}

// fakeRetroArch answers on a local UDP socket with the given replies, in order.
func fakeRetroArch(t *testing.T, replies ...string) net.Conn {
	t.Helper()
	server, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { server.Close() })

	go func() {
		buf := make([]byte, 256)
		_, client, err := server.ReadFrom(buf)
		if err != nil {
			return
		}
		for _, r := range replies {
			server.WriteTo([]byte(r), client)
		}
	}()

	conn, err := net.Dial("udp", server.LocalAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func TestReadMemorySkipsStaleReplies(t *testing.T) {
	conn := fakeRetroArch(t,
		"READ_CORE_MEMORY 2024284 01 02", // late reply to an earlier read
		"READ_CORE_MEMORY 80000a0 50 4F", // RetroArch uses lowercase hex
	)
	b, err := ReadMemory(conn, 0x080000A0, 2)
	if err != nil {
		t.Fatal(err)
	}
	if b[0] != 0x50 || b[1] != 0x4F {
		t.Errorf("got % X", b)
	}
}

func TestReadMemoryError(t *testing.T) {
	conn := fakeRetroArch(t, "READ_CORE_MEMORY 2024284 -1 no memory map defined")
	if _, err := ReadMemory(conn, 0x02024284, 2); err == nil {
		t.Error("expected error")
	}
}
