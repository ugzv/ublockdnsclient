package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"runtime"
)

func usage() {
	fmt.Fprintf(os.Stderr, `uBlock DNS CLI v%s

Usage:
  ublockdns install   -profile <id>   Install as system service and activate
                      [-server <url>] Optional DoH server base URL (for local/dev)
                      [-api-server <url>] Optional API server URL (for local/dev)
                      [-token <account-key>] Optional account key for instant rule-update cache flush
  ublockdns uninstall                  Remove service and restore DNS
  ublockdns start                      Start the service
  ublockdns stop                       Stop the service
  ublockdns run       -profile <id>    Run in foreground (for testing)
                      [-server <url>] Optional DoH server base URL (for local/dev)
                      [-api-server <url>] Optional API server URL (for local/dev)
                      [-token <account-key>] Optional account key for instant rule-update cache flush
  ublockdns status                     Show current status
  ublockdns version                    Print version

`, version)
}

func main() {
	if len(os.Args) < 2 {
		if runtime.GOOS == "windows" {
			fmt.Fprintln(os.Stderr, "uBlock DNS is a command-line tool.")
			fmt.Fprintln(os.Stderr, "For guided setup, run setup.ps1 as Administrator.")
		}
		usage()
		pauseBeforeExit()
		os.Exit(1)
	}

	cmd := os.Args[1]

	switch cmd {
	case "version":
		fmt.Printf("ublockdns-cli v%s\n", version)

	case "run":
		profileID := flagValue("-profile")
		dohServer := flagValue("-server")
		apiServer := flagValue("-api-server")
		token := flagValue("-token")
		if err := validateProfileID(profileID); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if err := run(profileID, dohServer, apiServer, token); err != nil {
			log.Fatalf("Error: %v", err)
		}

	case "install":
		profileID := flagValue("-profile")
		dohServer := flagValue("-server")
		apiServer := flagValue("-api-server")
		token := flagValue("-token")
		if err := validateProfileID(profileID); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if err := install(profileID, dohServer, apiServer, token); err != nil {
			log.Fatalf("Install failed: %v", err)
		}
		fmt.Println("uBlock DNS installed and activated.")
		fmt.Printf("All DNS queries now route through your profile: %s\n", profileID)

	case "uninstall":
		if err := uninstall(); err != nil {
			log.Fatalf("Uninstall failed: %v", err)
		}
		fmt.Println("uBlock DNS uninstalled. DNS restored to defaults.")

	case "start":
		if err := serviceStart(); err != nil {
			log.Fatalf("Start failed: %v", err)
		}
		fmt.Println("uBlock DNS started.")

	case "stop":
		if err := serviceStop(); err != nil {
			log.Fatalf("Stop failed: %v", err)
		}
		fmt.Println("uBlock DNS stopped.")

	case "status":
		showStatus()

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
			return os.Args[i+1]
		}
	}
	return ""
}
