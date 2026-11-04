package utils

import (
	"net/http/httptest"
	"testing"
)

func TestClientIP(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		headers    map[string]string
		want       string
	}{
		{name: "remote addr only", remoteAddr: "10.0.0.5:4321", want: "10.0.0.5"},
		{name: "ipv6 remote addr", remoteAddr: "[2001:db8::1]:4321", want: "2001:db8::1"},
		{name: "unparseable remote addr", remoteAddr: "garbage", want: "garbage"},
		{
			name:       "x-forwarded-for chain uses first hop",
			remoteAddr: "10.0.0.5:4321",
			headers:    map[string]string{"X-Forwarded-For": " 203.0.113.7 , 10.0.0.1"},
			want:       "203.0.113.7",
		},
		{
			name:       "x-real-ip",
			remoteAddr: "10.0.0.5:4321",
			headers:    map[string]string{"X-Real-IP": "198.51.100.2"},
			want:       "198.51.100.2",
		},
		{
			name:       "x-forwarded-for wins over x-real-ip",
			remoteAddr: "10.0.0.5:4321",
			headers:    map[string]string{"X-Forwarded-For": "203.0.113.7", "X-Real-IP": "198.51.100.2"},
			want:       "203.0.113.7",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			r.RemoteAddr = tt.remoteAddr
			for k, v := range tt.headers {
				r.Header.Set(k, v)
			}
			if got := ClientIP(r); got != tt.want {
				t.Errorf("ClientIP() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetLocalIP(t *testing.T) {
	ip, err := GetLocalIP()
	if err != nil {
		t.Skipf("no usable network address in this environment: %v", err)
	}
	if ip.IsLoopback() {
		t.Errorf("GetLocalIP() returned loopback address %s", ip)
	}
}
