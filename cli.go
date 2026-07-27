package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"runtime"
	"strings"
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
                      [-token-file <path>] Read the account token from a file instead of argv
  ublockdns uninstall                  Remove service and restore DNS
  ublockdns start                      Start the service
  ublockdns stop                       Stop the service
  ublockdns run       -profile <profile-id>    Run in foreground (for testing)
                      [-server <url>] Optional DoH server base URL (for local/dev)
                      [-api-server <url>] Optional API server URL (for local/dev)
                      [-token <account-token>] Optional account token for instant rules update cache flush
                      [-token-file <path>] Read the account token from a file instead of argv
  ublockdns upgrade   [-api-server <url>]  Update to the latest release and restart the service
  ublockdns status    [-json]          Show current status
  ublockdns wait-ready [-timeout <d>]  Wait until service and DNS are active
  ublockdns version                    Print version

`, version)
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
		args := mustParseProfileArgs()
		fmt.Println("Starting uBlockDNS in foreground...")
		if err := app_runtime.Run(version, args.profileID, args.dohServer, args.apiServer, args.token); err != nil {
			log.Fatalf("Error: %v", err)
		}

	case "install":
		args := mustParseProfileArgs()
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

	case "configure-system-dns":
		fmt.Println("Configuring durable Linux system DNS...")
		if err := core.ConfigureLinuxSystemDNS(); err != nil {
			log.Fatalf("configure-system-dns failed: %v", err)
		}
		fmt.Println("Linux system DNS configured.")

	case "uninstall":
		fmt.Println("Uninstalling uBlockDNS service...")
		result, err := service.Uninstall()
		if err != nil {
			log.Fatalf("Uninstall failed: %v", err)
		}
		for _, warning := range result.Warnings {
			fmt.Printf("Warning: %s\n", warning)
		}
		if len(result.Warnings) == 0 {
			fmt.Println("uBlockDNS service removed and system DNS restored.")
		} else {
			fmt.Println("uBlockDNS service removed. DNS may need manual cleanup — see warnings above.")
		}
		fmt.Println("The client binary was kept on disk. Remove it manually if you no longer need it.")

	case "upgrade":
		fmt.Println("Checking for updates...")
		v, err := service.Upgrade(version, app_runtime.ResolveAPIServer(flagValue("-api-server")))
		if err != nil {
			log.Fatalf("Upgrade failed: %v", err)
		}
		if v == "" {
			fmt.Printf("ublockdns v%s is up to date.\n", version)
		} else {
			fmt.Printf("Upgraded to v%s.\n", v)
		}

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

func mustParseProfileArgs() profileArgs {
	args, err := parseProfileArgs()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	return args
}

func parseProfileArgs() (profileArgs, error) {
	profileID, err := core.NormalizeProfileIDInput(flagValue("-profile"))
	if err != nil {
		return profileArgs{}, err
	}
	token, err := resolveTokenArg()
	if err != nil {
		return profileArgs{}, err
	}
	return profileArgs{
		profileID: profileID,
		dohServer: flagValue("-server"),
		apiServer: flagValue("-api-server"),
		token:     token,
	}, nil
}

// resolveTokenArg sources the account token, preferring the forms that keep it
// out of the process list, where any local user can read it. -token is still
// honoured first for backwards compatibility with existing scripts.
func resolveTokenArg() (string, error) {
	if token := strings.TrimSpace(flagValue("-token")); token != "" {
		return token, nil
	}
	if path := flagValue("-token-file"); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read token file %q: %w", path, err)
		}
		return strings.TrimSpace(string(data)), nil
	}
	return strings.TrimSpace(os.Getenv("UBLOCKDNS_ACCOUNT_TOKEN")), nil
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

// splitFlag normalizes one argument into a canonical single-dash flag name and
// its inline value, so "-profile x", "--profile x", "-profile=x" and
// "--profile=x" are all accepted. Non-flag arguments simply fail to match any
// known name.
func splitFlag(arg string) (name, value string, hasValue bool) {
	name = "-" + strings.TrimLeft(arg, "-")
	if key, v, ok := strings.Cut(name, "="); ok {
		return key, v, true
	}
	return name, "", false
}

// Flags are only read after the subcommand, which is always os.Args[1].
func flagValue(name string) string {
	for i := 2; i < len(os.Args); i++ {
		flag, value, hasValue := splitFlag(os.Args[i])
		if flag != name {
			continue
		}
		if hasValue {
			return value
		}
		if i+1 < len(os.Args) {
			if next, _, _ := splitFlag(os.Args[i+1]); isKnownFlag(next) {
				return ""
			}
			return os.Args[i+1]
		}
	}
	return ""
}

func flagPresent(name string) bool {
	for i := 2; i < len(os.Args); i++ {
		if flag, _, _ := splitFlag(os.Args[i]); flag == name {
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
	case "-profile", "-server", "-api-server", "-token", "-token-file", "-json", "-timeout":
		return true
	default:
		return false
	}
}
