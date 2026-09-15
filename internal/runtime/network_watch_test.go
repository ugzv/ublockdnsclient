package runtime

import (
	"net"
	"testing"
)

func TestDiffInterfaces(t *testing.T) {
	tests := []struct {
		name string
		old  []interfaceState
		new  []interfaceState
		want string
	}{
		{
			name: "address changed with same interface",
			old:  []interfaceState{{Name: "eth0", Flags: net.FlagUp, Addrs: []net.Addr{strAddr("192.0.2.1/24")}}},
			new:  []interfaceState{{Name: "eth0", Flags: net.FlagUp, Addrs: []net.Addr{strAddr("192.0.2.2/24")}}},
			want: "eth0 192.0.2.1/24 removed",
		},
		{
			name: "address order unchanged",
			old:  []interfaceState{{Name: "eth0", Addrs: []net.Addr{strAddr("192.0.2.1/24"), strAddr("2001:db8::1/64")}}},
			new:  []interfaceState{{Name: "eth0", Addrs: []net.Addr{strAddr("2001:db8::1/64"), strAddr("192.0.2.1/24")}}},
		},
		{
			name: "empty",
			old:  nil,
			new:  nil,
			want: "",
		},
		{
			name: "new interface",
			old:  []interfaceState{},
			new: []interfaceState{
				{Name: "eth0"},
			},
			want: "eth0 added",
		},
		{
			name: "new interface inserted",
			old: []interfaceState{
				{Name: "lo"},
			},
			new: []interfaceState{
				{Name: "lo"},
				{Name: "eth0"},
			},
			want: "eth0 added",
		},
		{
			name: "interface removed",
			old: []interfaceState{
				{Name: "eth0"},
			},
			new:  []interfaceState{},
			want: "eth0 removed",
		},
		{
			name: "interface removed head",
			old: []interfaceState{
				{Name: "eth0"},
				{Name: "lo"},
			},
			new: []interfaceState{
				{Name: "lo"},
			},
			want: "eth0 removed",
		},
		{
			name: "interface up",
			old: []interfaceState{
				{Name: "eth0"},
			},
			new: []interfaceState{
				{Name: "eth0", Flags: net.FlagUp},
			},
			want: "eth0 up",
		},
		{
			name: "interface down",
			old: []interfaceState{
				{Name: "eth0", Flags: net.FlagUp},
			},
			new: []interfaceState{
				{Name: "eth0"},
			},
			want: "eth0 down",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := diffInterfaces(tt.old, tt.new); got != tt.want {
				t.Errorf("diffInterfaces() = %q, want %q", got, tt.want)
			}
		})
	}
}

type strAddr string

func (addr strAddr) String() string { return string(addr) }
func (strAddr) Network() string     { return "" }

func TestDiffAddrs(t *testing.T) {
	tests := []struct {
		name     string
		oldAddrs []net.Addr
		newAddrs []net.Addr
		want     string
	}{
		{
			name:     "addr added",
			oldAddrs: []net.Addr{strAddr("a")},
			newAddrs: []net.Addr{strAddr("a"), strAddr("b")},
			want:     "b added",
		},
		{
			name:     "addr removed",
			oldAddrs: []net.Addr{strAddr("a"), strAddr("b")},
			newAddrs: []net.Addr{strAddr("a")},
			want:     "b removed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := diffAddrs(tt.oldAddrs, tt.newAddrs); got != tt.want {
				t.Errorf("diffAddrs() = %q, want %q", got, tt.want)
			}
		})
	}
}
