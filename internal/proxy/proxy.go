// Package proxy implements a minimal local HTTP CONNECT proxy that dials every
// upstream connection with a fixed address family. Pointing HTTPS_PROXY at it
// forces a client (e.g. Claude Code) onto IPv4 without touching system
// routing tables.
//
// This is the per-process mechanism of choice because Bun-based clients bypass
// /etc/hosts and Node DNS ordering flags, but do honor HTTPS_PROXY.
package proxy

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"
)

// ConnRecord summarizes one tunneled connection.
type ConnRecord struct {
	Time   time.Time
	Host   string // requested host:port
	Remote string // actual upstream ip:port we dialed
	Up     int64
	Down   int64
	Dur    time.Duration
	Err    string
}

// Server is a CONNECT proxy bound to a local address.
type Server struct {
	network string // "tcp4" or "tcp6"
	ln      net.Listener
	logf    func(format string, args ...any)

	mu    sync.Mutex
	conns []ConnRecord
}

// Listen starts a proxy on addr (e.g. "127.0.0.1:0") that dials upstreams
// using network ("tcp4" or "tcp6"). logf may be nil.
func Listen(addr, network string, logf func(string, ...any)) (*Server, error) {
	if network != "tcp4" && network != "tcp6" {
		return nil, fmt.Errorf("proxy: unsupported network %q", network)
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("proxy: listen %s: %w", addr, err)
	}
	if logf == nil {
		logf = func(string, ...any) {}
	}
	s := &Server{network: network, ln: ln, logf: logf}
	go s.serve()
	return s, nil
}

// Addr returns the bound address, e.g. "127.0.0.1:51234".
func (s *Server) Addr() string { return s.ln.Addr().String() }

// URL returns the proxy URL suitable for HTTPS_PROXY.
func (s *Server) URL() string { return "http://" + s.Addr() }

// Close stops accepting new connections.
func (s *Server) Close() error { return s.ln.Close() }

// Records returns a copy of all connection records so far.
func (s *Server) Records() []ConnRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]ConnRecord, len(s.conns))
	copy(out, s.conns)
	return out
}

func (s *Server) record(r ConnRecord) {
	s.mu.Lock()
	s.conns = append(s.conns, r)
	s.mu.Unlock()
}

func (s *Server) serve() {
	for {
		c, err := s.ln.Accept()
		if err != nil {
			return
		}
		go s.handle(c)
	}
}

func (s *Server) handle(c net.Conn) {
	defer c.Close()
	br := bufio.NewReader(c)
	req, err := http.ReadRequest(br)
	if err != nil {
		return
	}
	if req.Method != http.MethodConnect {
		_, _ = io.WriteString(c, "HTTP/1.1 405 Method Not Allowed\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
		return
	}
	start := time.Now()
	up, err := net.DialTimeout(s.network, req.Host, 20*time.Second)
	if err != nil {
		_, _ = io.WriteString(c, "HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
		s.record(ConnRecord{Time: start, Host: req.Host, Err: err.Error()})
		s.logf("CONNECT %s FAILED (%s): %v", req.Host, s.network, err)
		return
	}
	defer up.Close()
	if _, err := io.WriteString(c, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		return
	}
	rec := ConnRecord{Time: start, Host: req.Host, Remote: up.RemoteAddr().String()}
	upDone := make(chan int64, 1)
	go func() {
		n, _ := io.Copy(up, br)
		if t, ok := up.(*net.TCPConn); ok {
			_ = t.CloseWrite()
		}
		upDone <- n
	}()
	down, _ := io.Copy(c, up)
	if t, ok := c.(*net.TCPConn); ok {
		_ = t.CloseWrite()
	}
	rec.Up = <-upDone
	rec.Down = down
	rec.Dur = time.Since(start)
	s.record(rec)
	s.logf("CONNECT %s -> %s up=%dB down=%dB dur=%.1fs",
		rec.Host, rec.Remote, rec.Up, rec.Down, rec.Dur.Seconds())
}
