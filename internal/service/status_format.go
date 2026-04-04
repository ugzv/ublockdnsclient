package service

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

func ShowStatus() {
	showStatus(CurrentStatus())
}

func ShowStatusInfo(info StatusInfo) {
	showStatus(info)
}

func showStatus(info StatusInfo) {
	fmt.Printf("Status: %s\n", info.Status)

	if len(info.SystemDNS) == 0 {
		fmt.Println("System DNS: (none)")
	} else if info.LocalDNS {
		fmt.Printf("System DNS: %s (includes 127.0.0.1 via uBlockDNS)\n", strings.Join(info.SystemDNS, ", "))
	} else {
		fmt.Printf("System DNS: %s\n", strings.Join(info.SystemDNS, ", "))
	}

	fmt.Printf("Service: %s\n", info.Service)
	if info.ReadyCode != "" {
		fmt.Printf("Readiness: %s\n", info.ReadyCode)
	}
	if info.ReadyDetail != "" {
		fmt.Printf("Detail: %s\n", info.ReadyDetail)
	}
	if info.ProbeError != "" {
		fmt.Printf("Probe error: %s\n", info.ProbeError)
	}
	for _, warning := range info.Warnings {
		fmt.Printf("Warning: %s\n", warning)
	}
}

func writeStatusJSON(w io.Writer, info StatusInfo) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(info)
}

func WriteStatusJSON(info StatusInfo) error {
	return writeStatusJSON(os.Stdout, info)
}

func ShowStatusJSON() error {
	return WriteStatusJSON(CurrentStatus())
}
