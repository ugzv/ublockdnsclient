package core

func HasDNS127001(dns []string) bool {
	for _, d := range dns {
		if d == "127.0.0.1" {
			return true
		}
	}
	return false
}
