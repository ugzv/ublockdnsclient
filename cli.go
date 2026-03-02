package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"runtime"

	"github.com/ugzv/ublockdnsclient/internal/app"
)

func usage() {
	fmt.Fprintf(os.Stderr, `uBlockDNS CLI v%s

Usage:
  ublockdns install   -profile <profile-id>   Install as system service and activate
                      [-server <url>] Optional DoH server base URL (for local/dev)
                      [-api-server <url>] Optional API server URL (for local/dev)
                      [-token <account-token>] Optional account token for instant rules update cache flush
  ublockdns uninstall                  Remove service and restore DNS
  ublockdns start                      Start the service
  ublockdns stop                       Stop the service
  ublockdns run       -profile <profile-id>    Run in foreground (for testing)
                      [-server <url>] Optional DoH server base URL (for local/dev)
                      [-api-server <url>] Optional API server URL (for local/dev)
                      [-token <account-token>] Optional account token for instant rules update cache flush
  ublockdns status                     Show current status
  ublockdns version                    Print version

`, version)
}

func main() {
	if len(os.Args) < 2 {
		if runtime.GOOS == "windows" {
			fmt.Fprintln(os.Stderr, "uBlockDNS is a command-line tool.")
			fmt.Fprintln(os.Stderr, "For guided setup, run: powershell -ExecutionPolicy Bypass -File \"$env:ProgramFiles\\uBlockDNS\\setup.ps1\"")
		}
		usage()
		pauseBeforeExit()
		os.Exit(1)
	}

	cmd := os.Args[1]

	switch cmd {
	case "version":
		fmt.Printf("ublockdns v%s\n", version)

	case "run":
		profileID := flagValue("-profile")
		dohServer := flagValue("-server")
		apiServer := flagValue("-api-server")
		token := flagValue("-token")
		normalizedProfileID, err := app.NormalizeProfileIDInput(profileID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Starting uBlockDNS in foreground...")
		if err := app.Run(version, normalizedProfileID, dohServer, apiServer, token); err != nil {
			log.Fatalf("Error: %v", err)
		}

	case "install":
		profileID := flagValue("-profile")
		dohServer := flagValue("-server")
		apiServer := flagValue("-api-server")
		token := flagValue("-token")
		normalizedProfileID, err := app.NormalizeProfileIDInput(profileID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Installing uBlockDNS service...")
		if err := app.Install(normalizedProfileID, dohServer, apiServer, token); err != nil {
			log.Fatalf("Install failed: %v", err)
		}
		fmt.Println("uBlockDNS installed and activated.")
		fmt.Printf("All DNS queries now route through your profile: %s\n", normalizedProfileID)

	case "uninstall":
		fmt.Println("Uninstalling uBlockDNS service...")
		if err := app.Uninstall(); err != nil {
			log.Fatalf("Uninstall failed: %v", err)
		}
		fmt.Println("uBlockDNS uninstalled. DNS restored to defaults.")

	case "start":
		fmt.Println("Starting uBlockDNS service...")
		if err := app.ServiceStart(); err != nil {
			log.Fatalf("Start failed: %v", err)
		}
		fmt.Println("uBlockDNS started.")

	case "stop":
		fmt.Println("Stopping uBlockDNS service...")
		if err := app.ServiceStop(); err != nil {
			log.Fatalf("Stop failed: %v", err)
		}
		fmt.Println("uBlockDNS stopped.")

	case "status":
		app.ShowStatus()

	default:
		usage()
		pauseBeforeExit()
		os.Exit(1)
	}
}

func pauseBeforeExit() {
	if runtime.GOOS != "windows" {
		return
	}
	if os.Getenv("UBLOCKDNS_NO_PAUSE") == "1" {
		return
	}
	fmt.Fprintln(os.Stderr, "Press Enter to exit...")
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
}

func flagValue(name string) string {
	for i, arg := range os.Args {
		if arg == name && i+1 < len(os.Args) {
			if isKnownFlag(os.Args[i+1]) {
				return ""
			}
			return os.Args[i+1]
		}
	}
	return ""
}

func isKnownFlag(arg string) bool {
	switch arg {
	case "-profile", "-server", "-api-server", "-token":
		return true
	default:
		return false
	}
}
