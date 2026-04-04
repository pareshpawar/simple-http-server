package utils

import (
	"fmt"
	"net"
)

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
