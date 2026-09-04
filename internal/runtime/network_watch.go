// Network interface polling derived from github.com/nextdns/nextdns/netstatus.
// Kept instance-local (one goroutine per activation context) to avoid the global
// Notify/Stop singleton that races under go test -race.
package runtime

import (
	"context"
	"fmt"
	"net"
	"sort"
	"time"
)

var watchNetworkChanges = defaultWatchNetworkChanges

func defaultWatchNetworkChanges(ctx context.Context, changes chan<- string) {
	tick := time.NewTicker(10 * time.Second)
	defer tick.Stop()

	prev, _ := snapshotInterfaces()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			current, err := snapshotInterfaces()
			if err != nil {
				continue
			}
			if reason := diffInterfaces(prev, current); reason != "" {
				select {
				case changes <- reason:
				case <-ctx.Done():
					return
				}
			}
			prev = current
		}
	}
}

type interfaceState struct {
	Name  string
	Flags net.Flags
	Addrs []net.Addr
}

func snapshotInterfaces() ([]interfaceState, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	states := make([]interfaceState, 0, len(interfaces))
	for _, iface := range interfaces {
		addrs, err := iface.Addrs()
		if err != nil {
			return nil, err
		}
		states = append(states, interfaceState{Name: iface.Name, Flags: iface.Flags, Addrs: addrs})
	}
	return states, nil
}

func diffInterfaces(old, new []interfaceState) string {
	old = append([]interfaceState(nil), old...)
	new = append([]interfaceState(nil), new...)
	sort.Slice(old, func(i, j int) bool { return old[i].Name < old[j].Name })
	sort.Slice(new, func(i, j int) bool { return new[i].Name < new[j].Name })

	l := len(old)
	if l2 := len(new); l2 > l {
		l = l2
	}
	for i := 0; i < l; i++ {
		if len(old) <= i {
			return fmt.Sprintf("%s added", new[i].Name)
		}
		if len(new) <= i {
			return fmt.Sprintf("%s removed", old[i].Name)
		}
		if old[i].Name != new[i].Name {
			if old[i].Name < new[i].Name {
				return fmt.Sprintf("%s removed", old[i].Name)
			}
			return fmt.Sprintf("%s added", new[i].Name)
		}
		if old[i].Flags != new[i].Flags {
			oldUp := old[i].Flags&net.FlagUp != 0
			newUp := new[i].Flags&net.FlagUp != 0
			if oldUp != newUp {
				if oldUp && !newUp {
					return fmt.Sprintf("%s down", new[i].Name)
				}
				return fmt.Sprintf("%s up", new[i].Name)
			}
			return fmt.Sprintf("%s flag %v -> %v", new[i].Name, old[i].Flags, new[i].Flags)
		}
		if d := diffAddrs(old[i].Addrs, new[i].Addrs); d != "" {
			return fmt.Sprintf("%s %s", new[i].Name, d)
		}
	}
	return ""
}

func diffAddrs(oldAddrs, newAddrs []net.Addr) string {
oldIP:
	for _, oip := range oldAddrs {
		for _, nip := range newAddrs {
			if oip.String() == nip.String() {
				continue oldIP
			}
		}
		return fmt.Sprintf("%s removed", oip)
	}
	if len(oldAddrs) != len(newAddrs) {
	newIP:
		for _, nip := range newAddrs {
			for _, oip := range oldAddrs {
				if oip.String() == nip.String() {
					continue newIP
				}
			}
			return fmt.Sprintf("%s added", nip)
		}
	}
	return ""
}
