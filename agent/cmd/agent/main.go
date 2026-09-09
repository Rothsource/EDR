package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/kardianos/service"

	"khemstrix-agent/internal/config"
	"khemstrix-agent/internal/core"
	"khemstrix-agent/internal/platform"
)

var subcommands = map[string]bool{
	"install": true, "uninstall": true, "start": true, "stop": true, "status": true,
}

func main() {
	var action string
	var flagArgs []string

	if len(os.Args) >= 2 && subcommands[os.Args[1]] {
		action = os.Args[1]
		flagArgs = os.Args[2:]
	} else {
		flagArgs = os.Args[1:]
	}

	fs := flag.NewFlagSet("khemstrix-agent", flag.ExitOnError)
	server := fs.String("server", "", "EDR server URL")
	token := fs.String("token", "", "One-time enrollment token")
	fs.Parse(flagArgs)
	flags := config.Flags{Server: *server, Token: *token}

	svcConfig := platform.Config()
	// Do NOT bake CLI flags into the OS service definition. Leaving
	// Arguments empty ensures systemd and Windows SCM run the bare binary,
	// so config.json remains the authoritative source of truth across
	// reboots. This means the service process can NEVER perform first-time
	// registration itself (it never has --server/--token) — registration
	// must happen in the foreground, in autoInstallAndStart, before the
	// service is ever installed/started. See core.RegisterAndSaveConfig.
	svcConfig.Arguments = []string{}

	prg := platform.NewProgram(func(ctx context.Context) error {
		return core.Run(ctx, flags)
	})

	svc, err := service.New(prg, svcConfig)
	if err != nil {
		log.Fatalf("failed to initialize service: %v", err)
	}

	// service.Interactive() is false when launched by systemd or Windows SCM.
	if !service.Interactive() {
		if err := svc.Run(); err != nil {
			log.Fatalf("service run error: %v", err)
		}
		return
	}

	// From here down: an operator ran this binary directly from a terminal.
	if action != "" {
		switch action {
		case "install", "uninstall", "start", "stop":
			if err := service.Control(svc, action); err != nil {
				log.Fatalf("failed to %s service: %v", action, err)
			}
			fmt.Printf("service %s: OK\n", action)
		case "status":
			printStatus(svc)
		}
		return
	}

	// Check whether the agent has already been registered on this host —
	// AND, if the operator passed a --server flag this run, that it still
	// points at the same server. A flag that disagrees with the saved
	// config means the operator wants to re-point/re-register this agent,
	// not silently keep running against whatever it was registered to
	// before.
	existingCfg, cfgErr := config.Load()
	hasValidExistingConfig := cfgErr == nil && existingCfg != nil &&
		existingCfg.AgentID != "" && existingCfg.APIKey != ""
	serverMatches := *server == "" || (hasValidExistingConfig && existingCfg.Server == *server)
	hasExistingConfig := hasValidExistingConfig && serverMatches

	if hasValidExistingConfig && !serverMatches {
		fmt.Printf("Existing registration found for %s, but --server=%s was passed — re-registering against the new server.\n", existingCfg.Server, *server)
	}

	// Only require flags for fresh, unregistered (or re-pointed) installations.
	if !hasExistingConfig && (*server == "" || *token == "") {
		fmt.Println("usage: khemstrix-agent --server=<url> --token=<token>")
		fmt.Println("       (installs and starts itself as a background service automatically)")
		fmt.Println("advanced: khemstrix-agent install|uninstall|start|stop|status")
		os.Exit(1)
	}

	if err := autoInstallAndStart(svc, flagArgs, flags, hasExistingConfig); err != nil {
		fmt.Printf("Setup failed: %v\n", err)
		os.Exit(1)
	}
}

func autoInstallAndStart(svc service.Service, flagArgs []string, flags config.Flags, hasExistingConfig bool) error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("could not determine own path: %w", err)
	}

	target := persistentInstallPath()

	if filepath.Clean(exePath) != filepath.Clean(target) {
		fmt.Println("Installing to a permanent location...")
		if err := copyFile(exePath, target); err != nil {
			return fmt.Errorf("could not install to %s: %w", target, err)
		}
		if runtime.GOOS != "windows" {
			os.Chmod(target, 0755)
		}

		// Hand off to the installed copy with the original CLI arguments
		// so it can complete registration and trigger initial service creation.
		cmd := exec.Command(target, flagArgs...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("handoff to installed copy failed: %w", err)
		}
		return nil
	}

	// Already running from the permanent location.
	// Register now, in the foreground, if we don't already have valid
	// config for the requested server. This has to happen here — the
	// service is about to be launched with zero CLI arguments, so this
	// is the only point in the whole flow where flags.Server/flags.Token
	// are actually available to perform registration.
	if !hasExistingConfig {
		fmt.Println("Registering with the server...")
		if _, err := core.RegisterAndSaveConfig(flags.Server, flags.Token); err != nil {
			return fmt.Errorf("registration failed: %w", err)
		}
		fmt.Println("Registered successfully.")
	}

	fmt.Println("Registering as a background service...")
	// If the service is already installed, reinstall to clear out old baked arguments.
	_ = service.Control(svc, "uninstall")
	if err := service.Control(svc, "install"); err != nil {
		return fmt.Errorf("service install failed: %w", err)
	}

	fmt.Println("Starting service...")
	if err := service.Control(svc, "start"); err != nil {
		return fmt.Errorf("service start failed: %w", err)
	}

	fmt.Println("Waiting for the agent to confirm it can reach the server...")
	for i := 0; i < 15; i++ {
		time.Sleep(2 * time.Second)
		state, err := config.LoadState()
		if err == nil && state.LastSuccessAt != "" && state.LastError == "" {
			fmt.Println()
			fmt.Println("SUCCESS — agent installed, running in the background, and connected.")
			printStatus(svc)
			return nil
		}
	}

	return fmt.Errorf("installed and started, but no successful check-in yet — run `%s status` to check, or inspect service logs", target)
}

func persistentInstallPath() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(`C:\Program Files\khemstrix-agent`, "khemstrixAgent.exe")
	}
	return "/opt/khemstrix-agent/khemstrixAgent"
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

func printStatus(svc service.Service) {
	osStatus, err := svc.Status()
	if err != nil {
		fmt.Println("service status: not installed")
	} else {
		switch osStatus {
		case service.StatusRunning:
			fmt.Println("service status: running")
		case service.StatusStopped:
			fmt.Println("service status: stopped")
		default:
			fmt.Println("service status: unknown")
		}
	}

	state, err := config.LoadState()
	if err != nil {
		fmt.Println("connectivity: no data yet")
		return
	}
	if state.LastError != "" {
		fmt.Printf("connectivity: LAST ATTEMPT FAILED — %s\n", state.LastError)
	} else if state.LastSuccessAt != "" {
		fmt.Printf("connectivity: OK — last successful check-in: %s\n", state.LastSuccessAt)
	}
}
