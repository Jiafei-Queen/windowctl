// remote.go — `windowctl remote`: share the screen AND hand over
// mouse/keyboard control.
//
// Everything that serves is now the library (windowctl.Remote): this
// file is only the command-line skin — flags in, printed links out,
// Ctrl-C to revoke. The one capability the library deliberately does
// NOT have is cloudflared, so `--tunnel` is implemented here, against
// the port the library reports.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	windowctl "github.com/Jiafei-Queen/windowctl"
)

func remoteCmd(args []string) {
	fs := flag.NewFlagSet("remote", flag.ExitOnError)
	monitor := fs.Int("monitor", 0, "monitor to share by default (>=1; omit for the focused one)")
	port := fs.Int("port", 0, "localhost port to bind (0 = pick a free one)")
	tunnel := fs.Bool("tunnel", false, "also expose a public URL via cloudflared (default: local URL only)")
	fps := fs.Float64("fps", 10, "target frames per second for the live stream (1-30)")
	key := fs.String("key", "", "shared access key gating every request (default: fresh random token per run). "+
		"Set a stable key when the viewer URL is stable — e.g. behind a named tunnel on your own domain — "+
		"so the link survives restarts. Falls back to $WINDOWCTL_REMOTE_KEY when the flag is empty.")
	_ = fs.Parse(args)

	// Resolve the access key: explicit --key wins, then the env var,
	// then (inside the library) a fresh random per-run token.
	accessKey := *key
	if accessKey == "" {
		accessKey = os.Getenv("WINDOWCTL_REMOTE_KEY")
	}

	monitorID, err := monitorIDFromFlag(fs, monitor)
	if err != nil {
		fmt.Fprintln(os.Stderr, "windowctl remote:", err)
		os.Exit(2)
	}

	h, err := windowctl.Remote(windowctl.RemoteOptions{
		Monitor: monitorID,
		Port:    *port,
		FPS:     *fps,
		Key:     accessKey,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "windowctl remote:", err)
		// A rejected key is bad user input (exit 2, as before the
		// server moved into the library); a bind failure or a dead
		// crypto/rand is an environment failure (exit 1).
		if accessKey != "" && len(accessKey) < 16 {
			os.Exit(2)
		}
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Println("windowctl remote — screen share + CONTROL is live.")
	fmt.Println("  Open from another device on your network (the link carries the access token):")
	fmt.Printf("\n    %s\n\n", h.URL)
	fmt.Printf("  On this machine: %s\n\n", h.LoopbackURL)

	var tunnelCmd *exec.Cmd
	if *tunnel {
		tunnelCmd, err = startTunnel(ctx, h.Port, h.Key)
		if err != nil {
			fmt.Fprintln(os.Stderr, "windowctl remote:", err)
			_ = h.Stop()
			os.Exit(1)
		}
	}

	fmt.Println("Control is LIVE for anyone with the link. Press Ctrl-C to revoke and stop.")
	<-ctx.Done()

	fmt.Println("\nwindowctl remote: shutting down…")
	_ = h.Stop()
	if tunnelCmd != nil && tunnelCmd.Process != nil {
		_ = tunnelCmd.Process.Signal(syscall.SIGTERM)
		_ = tunnelCmd.Wait()
	}
}

// startTunnel spawns cloudflared and waits for it to print the public
// URL (which it writes to stderr). The token is appended so the link
// we surface is directly openable.
//
// This stays in the CLI, not the library: windowctl.Remote serves a LAN
// address and has no way to publish it, which is what stops a program
// that merely links the library from putting a desktop-control endpoint
// on the public internet. Reaching the internet takes this extra,
// explicit, human-typed step.
func startTunnel(ctx context.Context, port int, token string) (*exec.Cmd, error) {
	if _, err := exec.LookPath("cloudflared"); err != nil {
		return nil, fmt.Errorf("cloudflared not found on PATH — install it (brew install cloudflared) or use --no-tunnel")
	}
	cmd := exec.CommandContext(ctx, "cloudflared", "tunnel", "--url", fmt.Sprintf("http://127.0.0.1:%d", port))
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting cloudflared: %w", err)
	}

	urlCh := make(chan string, 1)
	go scanForTunnelURL(stderr, urlCh)

	select {
	case publicURL := <-urlCh:
		fmt.Printf("  Public link (cloudflared — carries the same token):\n\n    %s/?t=%s\n\n", publicURL, token)
	case <-time.After(30 * time.Second):
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("cloudflared did not report a public URL within 30s")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return cmd, nil
}
