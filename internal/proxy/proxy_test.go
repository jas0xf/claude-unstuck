package proxy

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

// TestConnectTunnel verifies CONNECT handshake, byte counting, and that the
// upstream dial uses the forced family (tcp4 → 127.0.0.1).
func TestConnectTunnel(t *testing.T) {
	// Upstream echo server on IPv4 loopback.
	up, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer up.Close()
	go func() {
		c, err := up.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		_, _ = io.Copy(c, c)
	}()

	srv, err := Listen("127.0.0.1:0", "tcp4", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	c, err := net.Dial("tcp", srv.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	fmt.Fprintf(c, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", up.Addr(), up.Addr())
	br := bufio.NewReader(c)
	line, err := br.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(line, "200") {
		t.Fatalf("CONNECT response = %q, want 200", line)
	}
	// Skip remaining header lines until blank.
	for {
		l, err := br.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if l == "\r\n" {
			break
		}
	}
	msg := "hello-over-tunnel"
	if _, err := io.WriteString(c, msg); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, len(msg))
	c.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, err := io.ReadFull(br, buf); err != nil {
		t.Fatal(err)
	}
	if string(buf) != msg {
		t.Fatalf("echo = %q, want %q", buf, msg)
	}
	c.Close()

	deadline := time.Now().Add(3 * time.Second)
	for {
		recs := srv.Records()
		if len(recs) == 1 && recs[0].Err == "" {
			r := recs[0]
			if !strings.HasPrefix(r.Remote, "127.0.0.1:") {
				t.Fatalf("upstream remote = %s, want 127.0.0.1 (tcp4 forced)", r.Remote)
			}
			if r.Up != int64(len(msg)) || r.Down != int64(len(msg)) {
				t.Fatalf("bytes up=%d down=%d, want %d/%d", r.Up, r.Down, len(msg), len(msg))
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("connection record not finalized: %+v", recs)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestConnectTunnelV6 proves the proxy dials the upstream over IPv6 when locked
// to tcp6 — the upstream socket lands on ::1, never a v4 address. This is the
// portable, deterministic version of the "doctor's v6 arm really uses v6" claim.
func TestConnectTunnelV6(t *testing.T) {
	up, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Skipf("no IPv6 loopback on this host: %v", err)
	}
	defer up.Close()
	go func() {
		c, err := up.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		_, _ = io.Copy(c, c)
	}()

	srv, err := Listen("127.0.0.1:0", "tcp6", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	c, err := net.Dial("tcp", srv.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	fmt.Fprintf(c, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", up.Addr(), up.Addr())
	br := bufio.NewReader(c)
	line, _ := br.ReadString('\n')
	if !strings.Contains(line, "200") {
		t.Fatalf("CONNECT response = %q, want 200", line)
	}
	c.Close()

	deadline := time.Now().Add(3 * time.Second)
	for {
		recs := srv.Records()
		if len(recs) == 1 {
			if !strings.HasPrefix(recs[0].Remote, "[::1]:") {
				t.Fatalf("upstream remote = %s, want [::1] (tcp6 forced)", recs[0].Remote)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("no connection record: %+v", recs)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestNoFamilyFallback proves there is no Happy-Eyeballs fallback: a tcp6-locked
// proxy asked to reach a v4-only address must FAIL (502), not silently connect
// over IPv4. This is why `doctor` can't report a false "v6 healthy".
func TestNoFamilyFallback(t *testing.T) {
	// A real v4 listener the proxy must refuse to reach over tcp6.
	up, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer up.Close()

	srv, err := Listen("127.0.0.1:0", "tcp6", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	c, err := net.Dial("tcp", srv.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	fmt.Fprintf(c, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", up.Addr(), up.Addr())
	c.SetReadDeadline(time.Now().Add(3 * time.Second))
	line, err := bufio.NewReader(c).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(line, "502") {
		t.Fatalf("tcp6 proxy reaching a v4 address: response = %q, want 502 (no fallback)", line)
	}
}

// TestNonConnectRejected verifies plain HTTP requests get 405.
func TestNonConnectRejected(t *testing.T) {
	srv, err := Listen("127.0.0.1:0", "tcp4", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	c, err := net.Dial("tcp", srv.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	fmt.Fprintf(c, "GET http://example.com/ HTTP/1.1\r\nHost: example.com\r\n\r\n")
	c.SetReadDeadline(time.Now().Add(3 * time.Second))
	line, err := bufio.NewReader(c).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(line, "405") {
		t.Fatalf("response = %q, want 405", line)
	}
}

// TestBadNetworkRejected verifies the family argument is validated.
func TestBadNetworkRejected(t *testing.T) {
	if _, err := Listen("127.0.0.1:0", "tcp", nil); err == nil {
		t.Fatal("expected error for network \"tcp\"")
	}
}
