package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"runtime"
	"time"

	"github.com/ugzv/ublockdnsclient/internal/core"
	app_runtime "github.com/ugzv/ublockdnsclient/internal/runtime"
	"github.com/ugzv/ublockdnsclient/internal/service"
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
  ublockdns status    [-json]          Show current status
  ublockdns wait-ready [-timeout <d>]  Wait until service and DNS are active
  ublockdns version                    Print version

`, version)
}

type profileCommandSpec struct {
	startMessage string
	failPrefix   string
	run          func(args profileArgs) error
	onSuccess    func(normalizedProfileID string)
}

type profileArgs struct {
	profileID string
	dohServer string
	apiServer string
	token     string
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
		executeProfileCommand(profileCommandSpec{
			startMessage: "Starting uBlockDNS in foreground...",
			failPrefix:   "Error",
			run: func(args profileArgs) error {
				return app_runtime.Run(version, args.profileID, args.dohServer, args.apiServer, args.token)
			},
		})

	case "install":
		args, err := parseProfileArgs()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Installing uBlockDNS service...")
		outcome, err := service.InstallDetailed(args.profileID, args.dohServer, args.apiServer, args.token)
		if err != nil {
			log.Fatalf("Install failed: %v", err)
		}
		switch outcome {
		case service.InstallOutcomeSwitched:
			fmt.Println("uBlockDNS profile switched and activated.")
		case service.InstallOutcomeUpdated:
			fmt.Println("uBlockDNS updated and activated.")
		default:
			fmt.Println("uBlockDNS installed and activated.")
		}
		fmt.Printf("All DNS queries now route through your profile: %s\n", args.profileID)

	case "uninstall":
		fmt.Println("Uninstalling uBlockDNS service...")
		if err := service.Uninstall(); err != nil {
			log.Fatalf("Uninstall failed: %v", err)
		}
		fmt.Println("uBlockDNS uninstalled. DNS restored to defaults.")

	case "start":
		fmt.Println("Starting uBlockDNS service...")
		if err := service.ServiceStart(); err != nil {
			log.Fatalf("Start failed: %v", err)
		}
		fmt.Println("uBlockDNS started.")

	case "stop":
		fmt.Println("Stopping uBlockDNS service...")
		if err := service.ServiceStop(); err != nil {
			log.Fatalf("Stop failed: %v", err)
		}
		fmt.Println("uBlockDNS stopped.")

	case "status":
		if flagPresent("-json") {
			if err := service.ShowStatusJSON(); err != nil {
				log.Fatalf("Status failed: %v", err)
			}
			return
		}
		service.ShowStatus()

	case "wait-ready":
		timeout, err := parseDurationFlag("-timeout", 45*time.Second)
		if err != nil {
			log.Fatalf("wait-ready failed: %v", err)
		}
		info, err := service.WaitUntilReady(timeout)
		if err != nil {
			if flagPresent("-json") {
				if jsonErr := service.WriteStatusJSON(info); jsonErr != nil {
					log.Printf("Status failed: %v", jsonErr)
					os.Exit(1)
				}
				os.Exit(1)
			} else {
				service.ShowStatusInfo(info)
			}
			fmt.Fprintf(os.Stderr, "wait-ready failed: %v\n", err)
			os.Exit(1)
		}
		if flagPresent("-json") {
			if err := service.WriteStatusJSON(info); err != nil {
				log.Fatalf("wait-ready failed: %v", err)
			}
			return
		}
		service.ShowStatusInfo(info)
		if info.Ready {
			fmt.Println("uBlockDNS is ready.")
		}

	default:
		usage()
		pauseBeforeExit()
		os.Exit(1)
	}
}

func executeProfileCommand(spec profileCommandSpec) {
	args, err := parseProfileArgs()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if spec.startMessage != "" {
		fmt.Println(spec.startMessage)
	}
	if err := spec.run(args); err != nil {
		log.Fatalf("%s: %v", spec.failPrefix, err)
	}
	if spec.onSuccess != nil {
		spec.onSuccess(args.profileID)
	}
}

func parseProfileArgs() (profileArgs, error) {
	profileID, err := core.NormalizeProfileIDInput(flagValue("-profile"))
	if err != nil {
		return profileArgs{}, err
	}
	return profileArgs{
		profileID: profileID,
		dohServer: flagValue("-server"),
		apiServer: flagValue("-api-server"),
		token:     flagValue("-token"),
	}, nil
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

func flagPresent(name string) bool {
	for _, arg := range os.Args {
		if arg == name {
			return true
		}
	}
	return false
}

func parseDurationFlag(name string, fallback time.Duration) (time.Duration, error) {
	raw := flagValue(name)
	if raw == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid %s value %q: %w", name, raw, err)
	}
	return d, nil
}

func isKnownFlag(arg string) bool {
	switch arg {
	case "-profile", "-server", "-api-server", "-token", "-json", "-timeout":
		return true
	default:
		return false
	}
}
