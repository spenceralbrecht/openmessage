package cmd

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/mdp/qrterminal/v3"
	"github.com/rs/zerolog"
	qr "rsc.io/qr"

	"github.com/maxghenis/openmessage/internal/app"
	"github.com/maxghenis/openmessage/internal/whatsapplive"
)

func RunWhatsApp(logger zerolog.Logger, args ...string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: openmessage whatsapp <status|connect|qr|unpair>")
	}
	switch args[0] {
	case "status":
		return runWhatsAppStatus(logger, args[1:]...)
	case "connect", "pair":
		return runWhatsAppConnect(logger, args[1:]...)
	case "qr":
		return runWhatsAppQR(logger, args[1:]...)
	case "unpair":
		return runWhatsAppUnpair(logger, args[1:]...)
	default:
		return fmt.Errorf("unknown whatsapp command %q; usage: openmessage whatsapp <status|connect|qr|unpair>", args[0])
	}
}

func runWhatsAppStatus(logger zerolog.Logger, args ...string) error {
	fs := flag.NewFlagSet("whatsapp status", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	connect := fs.Bool("connect", false, "attempt to connect an already-paired WhatsApp session before reporting status")
	wait := fs.Duration("wait", 5*time.Second, "maximum time to wait for a paired session to connect")
	if err := fs.Parse(args); err != nil {
		return err
	}

	a, err := app.New(logger)
	if err != nil {
		return fmt.Errorf("init app: %w", err)
	}
	defer a.Close()

	if *connect {
		if err := a.LoadAndConnectWhatsApp(); err != nil {
			return fmt.Errorf("connect whatsapp: %w", err)
		}
		waitForWhatsApp(a, *wait)
	}

	return writeJSON(os.Stdout, map[string]any{
		"whatsapp": a.WhatsAppStatus(),
	})
}

func runWhatsAppConnect(logger zerolog.Logger, args ...string) error {
	fs := flag.NewFlagSet("whatsapp connect", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	wait := fs.Duration("wait", 90*time.Second, "maximum time to wait for pairing or connection")
	startTimeout := fs.Duration("start-timeout", 20*time.Second, "maximum time to wait for WhatsApp bridge startup")
	qrPNGPath := fs.String("qr-png", "", "optional path to write the latest WhatsApp QR code as a PNG")
	jsonOnly := fs.Bool("json", false, "print status snapshots as JSON only")
	if err := fs.Parse(args); err != nil {
		return err
	}

	a, err := app.New(logger)
	if err != nil {
		return fmt.Errorf("init app: %w", err)
	}
	defer a.Close()

	if err := startWhatsAppConnectWithTimeout(a, *startTimeout); err != nil {
		return fmt.Errorf("start whatsapp connect: %w", err)
	}

	status, err := waitForWhatsAppConnect(a, *wait, !*jsonOnly, *qrPNGPath)
	if err != nil {
		if status.LastError != "" {
			return fmt.Errorf("%w: %s", err, status.LastError)
		}
		return err
	}
	return writeJSON(os.Stdout, map[string]any{
		"whatsapp": status,
	})
}

func startWhatsAppConnectWithTimeout(a *app.App, timeout time.Duration) error {
	done := make(chan error, 1)
	go func() {
		done <- a.StartWhatsAppConnect()
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		return fmt.Errorf("timed out after %s before WhatsApp returned a pairing state", timeout)
	}
}

func runWhatsAppQR(logger zerolog.Logger, args ...string) error {
	fs := flag.NewFlagSet("whatsapp qr", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	jsonOnly := fs.Bool("json", false, "print QR snapshot as JSON only")
	if err := fs.Parse(args); err != nil {
		return err
	}

	a, err := app.New(logger)
	if err != nil {
		return fmt.Errorf("init app: %w", err)
	}
	defer a.Close()

	snap, err := a.WhatsAppQRCode()
	if err != nil {
		return err
	}
	if !*jsonOnly {
		displayWhatsAppQR(snap)
	}
	return writeJSON(os.Stdout, map[string]any{
		"whatsapp_qr": snap,
	})
}

func runWhatsAppUnpair(logger zerolog.Logger, args ...string) error {
	fs := flag.NewFlagSet("whatsapp unpair", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	confirm := fs.Bool("confirm", false, "required confirmation to remove the local WhatsApp linked-device session")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if !*confirm {
		return fmt.Errorf("refusing to unpair WhatsApp without --confirm")
	}

	a, err := app.New(logger)
	if err != nil {
		return fmt.Errorf("init app: %w", err)
	}
	defer a.Close()

	if err := a.UnpairWhatsApp(); err != nil {
		return err
	}
	return writeJSON(os.Stdout, map[string]any{
		"unpaired": true,
	})
}

func waitForWhatsApp(a *app.App, wait time.Duration) whatsapplive.StatusSnapshot {
	deadline := time.Now().Add(wait)
	status := a.WhatsAppStatus()
	for time.Now().Before(deadline) {
		status = a.WhatsAppStatus()
		if status.Connected || status.LastError != "" || (!status.Connecting && !status.Pairing) {
			return status
		}
		time.Sleep(250 * time.Millisecond)
	}
	return a.WhatsAppStatus()
}

func waitForWhatsAppConnect(a *app.App, wait time.Duration, showQR bool, qrPNGPath string) (whatsapplive.StatusSnapshot, error) {
	deadline := time.Now().Add(wait)
	var displayedQRUpdatedAt int64
	var lastStatus whatsapplive.StatusSnapshot

	for time.Now().Before(deadline) {
		status := a.WhatsAppStatus()
		lastStatus = status
		if status.Connected {
			return status, nil
		}
		if status.LastError != "" && !status.Connecting && !status.Pairing {
			return status, fmt.Errorf("whatsapp connect failed")
		}
		if showQR && status.QRAvailable && status.QRUpdatedAt != displayedQRUpdatedAt {
			if snap, err := a.WhatsAppQRCode(); err == nil {
				displayWhatsAppQR(snap)
				if qrPNGPath != "" {
					if err := writeWhatsAppQRPNG(snap, qrPNGPath); err != nil {
						fmt.Fprintln(os.Stderr, "Failed to write QR PNG:", err)
					} else {
						fmt.Println("PNG:", qrPNGPath)
					}
				}
				displayedQRUpdatedAt = status.QRUpdatedAt
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return lastStatus, fmt.Errorf("whatsapp connect timed out after %s", wait)
}

func displayWhatsAppQR(snap whatsapplive.QRSnapshot) {
	fmt.Println("\nScan this QR code with WhatsApp:")
	fmt.Println("(WhatsApp > Settings > Linked Devices > Link a Device)")
	fmt.Println()
	qrterminal.GenerateHalfBlock(snap.Code, qrterminal.L, os.Stdout)
	fmt.Println()
	if snap.ExpiresAt > 0 {
		fmt.Println("Expires at:", time.UnixMilli(snap.ExpiresAt).Format(time.RFC3339))
	}
}

func writeWhatsAppQRPNG(snap whatsapplive.QRSnapshot, path string) error {
	code, err := qr.Encode(snap.Code, qr.M)
	if err != nil {
		return fmt.Errorf("encode WhatsApp QR: %w", err)
	}
	if err := os.WriteFile(path, code.PNG(), 0o600); err != nil {
		return fmt.Errorf("write WhatsApp QR PNG: %w", err)
	}
	return nil
}
