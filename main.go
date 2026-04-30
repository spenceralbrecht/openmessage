package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/rs/zerolog"

	"github.com/maxghenis/openmessage/cmd"
)

// version is set at build time via -ldflags "-X main.version=v0.2.0".
// Defaults to "dev" for local builds.
var version = "dev"

func main() {
	cmd.SetVersion(version)

	level := cmd.LogLevel()
	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).
		With().Timestamp().Logger().Level(level)

	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: openmessage <pair|serve|demo|conversations|messages|search|draft|drafts|send-draft|whatsapp|import>")
		fmt.Fprintln(os.Stderr, "  pair [--google|--google-file path]       - Pair with your phone via QR or Google account cookies")
		fmt.Fprintln(os.Stderr, "  serve [--demo|--mock]                    - Start the local web UI and MCP transports")
		fmt.Fprintln(os.Stderr, "  demo                                     - Start a seeded fake-data UI with live transports disabled")
		fmt.Fprintln(os.Stderr, "  conversations [--limit n]                - List cached conversations as JSON")
		fmt.Fprintln(os.Stderr, "  messages <conversation_id> [--limit n]    - List cached messages as JSON")
		fmt.Fprintln(os.Stderr, "  search <query> [--limit n]                - Search cached messages as JSON")
		fmt.Fprintln(os.Stderr, "  draft <conversation_id> <msg>             - Create a local draft as JSON")
		fmt.Fprintln(os.Stderr, "  drafts <conversation_id>                  - List local drafts for a conversation")
		fmt.Fprintln(os.Stderr, "  send-draft <draft_id> --confirm <id>      - Send a reviewed draft")
		fmt.Fprintln(os.Stderr, "  whatsapp <status|connect|qr|unpair>       - Manage WhatsApp linked-device setup")
		fmt.Fprintln(os.Stderr, "  send <conversation_id> <msg>              - Unsafe direct send, requires OPENMESSAGES_ALLOW_DIRECT_SEND=1")
		fmt.Fprintln(os.Stderr, "  send-group <phone1,phone2,...> <msg>       - Unsafe direct group send, requires OPENMESSAGES_ALLOW_DIRECT_SEND=1")
		fmt.Fprintln(os.Stderr, "  import gchat <groups-dir> [--email you@]  - Import Google Chat Takeout")
		fmt.Fprintln(os.Stderr, "  import gchat-conversation <messages.json> - Import single GChat conversation")
		fmt.Fprintln(os.Stderr, "  import imessage [db-path]                 - Import iMessage (needs Full Disk Access)")
		fmt.Fprintln(os.Stderr, "  import whatsapp <chat.txt> [--name You]   - Import WhatsApp text export")
		fmt.Fprintln(os.Stderr, "  import signal [support-dir]               - Import Signal Desktop history")
		os.Exit(1)
	}

	var err error
	switch os.Args[1] {
	case "pair":
		err = cmd.RunPair(logger, os.Args[2:]...)
	case "serve":
		err = cmd.RunServe(logger, os.Args[2:]...)
	case "demo":
		err = cmd.RunDemo(logger)
	case "conversations":
		err = cmd.RunConversations(logger, os.Args[2:]...)
	case "messages":
		err = cmd.RunMessages(logger, os.Args[2:]...)
	case "search":
		err = cmd.RunSearch(logger, os.Args[2:]...)
	case "draft":
		err = cmd.RunDraft(logger, os.Args[2:]...)
	case "drafts":
		err = cmd.RunDrafts(logger, os.Args[2:]...)
	case "send-draft":
		err = cmd.RunSendDraft(logger, os.Args[2:]...)
	case "whatsapp":
		err = cmd.RunWhatsApp(logger, os.Args[2:]...)
	case "send":
		if len(os.Args) < 4 {
			fmt.Fprintln(os.Stderr, "Usage: openmessage send <conversation_id> <message>")
			os.Exit(1)
		}
		err = cmd.RunSend(logger, os.Args[2], strings.Join(os.Args[3:], " "))
	case "send-group":
		if len(os.Args) < 4 {
			fmt.Fprintln(os.Stderr, "Usage: openmessage send-group <phone1,phone2,...> <message>")
			os.Exit(1)
		}
		phones := strings.Split(os.Args[2], ",")
		err = cmd.RunSendGroup(logger, phones, strings.Join(os.Args[3:], " "))
	case "import":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: openmessage import <gchat|gchat-conversation|imessage|whatsapp|signal> [args...]")
			os.Exit(1)
		}
		err = cmd.RunImport(logger, os.Args[2], os.Args[3:])
	case "debug-media":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: openmessage debug-media <conversation_id>")
			os.Exit(1)
		}
		err = cmd.RunDebugMedia(logger, os.Args[2])
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		fmt.Fprintln(os.Stderr, "Usage: openmessage <pair|serve|demo|conversations|messages|search|draft|drafts|send-draft|whatsapp|import>")
		os.Exit(1)
	}

	if err != nil {
		logger.Fatal().Err(err).Msg("Fatal error")
	}
}
