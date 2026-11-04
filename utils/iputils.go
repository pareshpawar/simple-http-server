// Package utils contains small networking helpers used by the server.
package utils

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
)

// GetMyOutboundIP returns the local IP address the host would use to reach
// the public internet. No packets are sent: dialing UDP only selects a route.
func GetMyOutboundIP() (net.IP, error) {
	conn, err := net.Dial("udp", "1.1.1.1:80")
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	localAddr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok {
		return nil, fmt.Errorf("unexpected address type: %T", conn.LocalAddr())
	}
	return localAddr.IP, nil
}

// GetLocalIP returns the preferred outbound IP. When there is no route to the
// internet (air-gapped clusters, locked-down sandboxes) it falls back to the
// first non-loopback interface address, preferring IPv4.
func GetLocalIP() (net.IP, error) {
	if ip, err := GetMyOutboundIP(); err == nil {
		return ip, nil
	}

	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}
	var v6 net.IP
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok || ipNet.IP.IsLoopback() || !ipNet.IP.IsGlobalUnicast() {
			continue
		}
		if ip4 := ipNet.IP.To4(); ip4 != nil {
			return ip4, nil
		}
		if v6 == nil {
			v6 = ipNet.IP
		}
	}
	if v6 != nil {
		return v6, nil
	}
	return nil, errors.New("no non-loopback network address found")
}

// ClientIP makes a best-effort guess at the original client's IP address,
// honouring the X-Forwarded-For and X-Real-IP headers set by proxies and load
// balancers. These headers are client-controlled, so the result is
// informational only and must not be used for access control.
func ClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		first, _, _ := strings.Cut(xff, ",")
		if ip := strings.TrimSpace(first); ip != "" {
			return ip
		}
	}
	if xri := strings.TrimSpace(r.Header.Get("X-Real-IP")); xri != "" {
		return xri
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
