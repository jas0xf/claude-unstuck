// Package dnsutil resolves hostnames with an explicit address family.
package dnsutil

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"time"
)

// Family selects the address family used for resolution.
type Family string

const (
	IPv4 Family = "ip4"
	IPv6 Family = "ip6"
)

// Resolve returns the addresses of host for the given family. It never mixes
// families: callers get only A or only AAAA results.
func Resolve(ctx context.Context, host string, fam Family) ([]netip.Addr, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	ips, err := net.DefaultResolver.LookupIP(ctx, string(fam), host)
	if err != nil {
		return nil, fmt.Errorf("resolve %s (%s): %w", host, fam, err)
	}
	var out []netip.Addr
	for _, ip := range ips {
		if a, ok := netip.AddrFromSlice(ip); ok {
			out = append(out, a.Unmap())
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("resolve %s (%s): no records", host, fam)
	}
	return out, nil
}

// HasGlobalIPv6 reports whether any local interface carries a global unicast
// IPv6 address (2000::/3). Used to distinguish "no IPv6 on this network" from
// "IPv6 present but blocked locally".
func HasGlobalIPv6() bool {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return false
	}
	global := netip.MustParsePrefix("2000::/3")
	for _, a := range addrs {
		ipn, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		ip, ok := netip.AddrFromSlice(ipn.IP)
		if !ok {
			continue
		}
		ip = ip.Unmap()
		if ip.Is6() && global.Contains(ip) {
			return true
		}
	}
	return false
}

// ResolveAll resolves every host in hosts for the given family and returns the
// deduplicated union. Hosts that fail to resolve are reported in errs but do
// not abort the rest.
func ResolveAll(ctx context.Context, hosts []string, fam Family) ([]netip.Addr, []error) {
	seen := map[netip.Addr]bool{}
	var out []netip.Addr
	var errs []error
	for _, h := range hosts {
		addrs, err := Resolve(ctx, h, fam)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		for _, a := range addrs {
			if !seen[a] {
				seen[a] = true
				out = append(out, a)
			}
		}
	}
	return out, errs
}
