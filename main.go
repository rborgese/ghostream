//go:generate pkger

// Package main provides the full-featured server with configuration loading
// and communication between routines.
package main

import (
	"log"

	"gitlab.crans.org/nounous/ghostream/auth"
	"gitlab.crans.org/nounous/ghostream/internal/config"
	"gitlab.crans.org/nounous/ghostream/internal/monitoring"
	"gitlab.crans.org/nounous/ghostream/stream/forwarding"
	"gitlab.crans.org/nounous/ghostream/stream/srt"
	"gitlab.crans.org/nounous/ghostream/stream/webrtc"
	"gitlab.crans.org/nounous/ghostream/web"
)

func main() {
	// Configure logger
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalln("Failed to load configuration:", err)
	}

	// Init authentification
	authBackend, err := auth.New(&cfg.Auth)
	if err != nil {
		log.Fatalln("Failed to load authentification backend:", err)
	}
	if authBackend != nil {
		defer authBackend.Close()
	}

	// WebRTC session description channels
	remoteSdpChan := make(chan struct {
		StreamID          string
		RemoteDescription webrtc.SessionDescription
	})
	localSdpChan := make(chan webrtc.SessionDescription)

	// SRT channel for forwarding and webrtc
	forwardingChannel := make(chan srt.Packet, 65536)
	webrtcChannel := make(chan srt.Packet, 65536)

	// Start routines
	go forwarding.Serve(forwardingChannel, cfg.Forwarding)
	go monitoring.Serve(&cfg.Monitoring)
	go srt.Serve(&cfg.Srt, authBackend, forwardingChannel, webrtcChannel)
	go web.Serve(remoteSdpChan, localSdpChan, &cfg.Web)
	go webrtc.Serve(remoteSdpChan, localSdpChan, webrtcChannel, &cfg.WebRTC)

	// Wait for routines
	select {}
}
