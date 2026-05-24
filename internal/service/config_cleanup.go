package service

import (
	"log"
	"os"
	"path/filepath"
	"runtime"
)

func removeServiceConfigBestEffort() {
	for _, path := range serviceConfigPaths() {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			log.Printf("Warning: failed to remove service config %s: %v", path, err)
		}
	}
}

func serviceConfigPaths() []string {
	switch runtime.GOOS {
	case "windows":
		programFiles := os.Getenv("ProgramFiles")
		if programFiles == "" {
			return nil
		}
		return []string{
			filepath.Join(programFiles, "uBlockDNS", "ublockdns.conf"),
			filepath.Join(programFiles, "ublockdns", "ublockdns.conf"),
		}
	default:
		return []string{"/etc/ublockdns.conf"}
	}
}
