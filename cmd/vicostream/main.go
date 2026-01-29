package main

import (
	"context"
	"fmt"
	"log"
	"os"
	ossignal "os/signal"
	"syscall"

	"vico_home/native/internal/api"
	"vico_home/native/internal/config"
	sigclient "vico_home/native/internal/signal"
	"vico_home/native/internal/viewer"
	"vico_home/native/internal/webrtc"
)

const helpText = `vicostream - Stream H264 video from a VICO camera via WebRTC

Usage:
  vicostream [options]

The raw H264 stream is written to stdout. Pipe to ffplay or ffmpeg for
playback or recording.

Environment Variables (required):
  VICO_TOKEN  JWT authentication token from the VICO app
  VICO_SN     Camera serial number

Examples:
  # Live playback
  vicostream | ffplay -f h264 -

  # Record to MP4
  vicostream | ffmpeg -f h264 -i - -c copy output.mp4

Options:
  -h, --help  Show this help message
`

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "-h" || os.Args[1] == "--help") {
		fmt.Print(helpText)
		os.Exit(0)
	}

	log.SetOutput(os.Stderr)
	log.SetFlags(log.Ltime | log.Lmicroseconds)

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("[main] %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	ossignal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		log.Printf("[main] received %s, shutting down", sig)
		cancel()
	}()

	// Step 1: Fetch ticket
	apiClient := api.NewClient()
	log.Printf("[main] getting WebRTC ticket for %s", cfg.SerialNumber)
	ticket, err := apiClient.FetchTicket(cfg.Token, cfg.SerialNumber)
	if err != nil {
		log.Fatalf("[main] get ticket: %v", err)
	}
	log.Printf("[main] ticket obtained: id=%s signal=%s", ticket.ID, ticket.SignalServer)

	// Step 2: Create peer connection
	peer, err := webrtc.NewPeer(ticket.ICEServers, cfg.SerialNumber)
	if err != nil {
		log.Fatalf("[main] create peer: %v", err)
	}

	// Step 3: Add transceivers
	if err := peer.AddTransceivers(); err != nil {
		log.Fatalf("[main] add transceivers: %v", err)
	}

	// Step 4: Create viewer (implements domain.Handler)
	v := viewer.New(peer, cancel)

	// Step 5: Create signal client with viewer as handler
	sc := sigclient.NewClient(ticket, cfg.SerialNumber, v)

	// Step 6: Complete the circular dependency
	v.SetSignaler(sc)

	// Step 7: Set up track handler (H264 → stdout)
	peer.SetOnTrack(os.Stdout)

	// Step 8: Set up ICE candidate forwarding
	peer.SetOnICECandidate(func(sdpMid string, sdpMLineIndex int, candidate string) {
		sc.SendICECandidate(sdpMid, sdpMLineIndex, candidate)
	})

	// Step 9: Connect signaling (AUTH → JOIN_LIVE → PEER_IN → offer flow)
	if err := sc.Connect(); err != nil {
		log.Fatalf("[main] signal connect: %v", err)
	}

	<-ctx.Done()
	log.Printf("[main] shutting down")

	peer.Close()
	sc.Close()

	log.Printf("[main] done")
}
