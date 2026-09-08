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
	svcConfig.Arguments = flagArgs // baked in for every future automatic restart

	prg := platform.NewProgram(func(ctx context.Context) error {
		return core.Run(ctx, flags)
	})

	svc, err := service.New(prg, svcConfig)
	if err != nil {
		log.Fatalf("failed to initialize service: %v", err)
	}

	// service.Interactive() is the key check: false means the OS service
	// manager (systemd/SCM) launched us — this happens on every boot and
	// every crash-recovery restart. In that case we must NEVER try to
	// self-install again — just run the actual agent loop.
	if !service.Interactive() {
		if err := svc.Run(); err != nil {
			log.Fatalf("service run error: %v", err)
		}
		return
	}

	// From here down: a human ran this binary directly.

	if action != "" {
		// Explicit manual control — still available for you as the admin,
		// but customers never need to type these.
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

	if *server == "" || *token == "" {
		fmt.Println("usage: khemstrix-agent --server=<url> --token=<token>")
		fmt.Println("       (installs and starts itself as a background service automatically)")
		fmt.Println("advanced: khemstrix-agent install|uninstall|start|stop|status")
		os.Exit(1)
	}

	// This is the one-shot, no-further-commands-needed setup path.
	if err := autoInstallAndStart(svc, svcConfig); err != nil {
		fmt.Printf("Setup failed: %v\n", err)
		os.Exit(1)
	}
}

// autoInstallAndStart does everything in one call: relocate the binary
// somewhere permanent (so it survives /tmp being wiped on reboot), install
// it as a real OS service, start it, then poll for real proof of
// connectivity before reporting success — matching exactly what a customer
// running the one-liner needs, with zero further commands.
func autoInstallAndStart(svc service.Service, svcConfig *service.Config) error {
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

		// Hand off to the permanently-installed copy with the same
		// arguments, so kardianos/service registers the SERVICE pointing
		// at the permanent path, not this temporary one.
		cmd := exec.Command(target, os.Args[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("handoff to installed copy failed: %w", err)
		}
		return nil
	}

	// We're already running from the permanent location — do the real work.
	fmt.Println("Registering as a background service...")
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

	return fmt.Errorf("installed and started, but no successful check-in yet — run `%s status` to check, or re-run this binary directly (without install) to see live errors", target)
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
