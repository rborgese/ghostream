//go:generate pkger

package main

import (
	"log"
	"net"
	"strings"

	"github.com/spf13/viper"
	"gitlab.crans.org/nounous/ghostream/auth"
	"gitlab.crans.org/nounous/ghostream/internal/monitoring"
	"gitlab.crans.org/nounous/ghostream/stream/forwarding"
	"gitlab.crans.org/nounous/ghostream/stream/srt"
	"gitlab.crans.org/nounous/ghostream/stream/webrtc"
	"gitlab.crans.org/nounous/ghostream/web"
)

func loadConfiguration() {
	// Load configuration from environnement variables
	// Replace "." to "_" for nested structs
	// e.g. GHOSTREAM_LDAP_URI will apply to Config.LDAP.URI
	viper.SetEnvPrefix("ghostream")
	replacer := strings.NewReplacer(".", "_")
	viper.SetEnvKeyReplacer(replacer)
	viper.AutomaticEnv()

	// Load configuration file if exists
	viper.SetConfigName("ghostream.yml")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("$HOME/.ghostream")
	viper.AddConfigPath("/etc/ghostream")
	viper.AddConfigPath(".")
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// Config file not found, ignore and use defaults
			log.Print(err)
		} else {
			// Config file was found but another error was produced
			log.Fatal(err)
		}
	} else {
		// Config loaded
		log.Printf("Using config file: %s", viper.ConfigFileUsed())
	}

	// Define configuration default values
	viper.SetDefault("Auth.Backend", "Basic")
	viper.SetDefault("Auth.Basic.Credentials", map[string]string{
		// Demo user with password "demo"
		"demo": "$2b$15$LRnG3eIHFlYIguTxZOLH7eHwbQC/vqjnLq6nDFiHSUDKIU.f5/1H6",
	})
	viper.SetDefault("Auth.LDAP.URI", "ldap://127.0.0.1:389")
	viper.SetDefault("Auth.LDAP.UserDn", "cn=users,dc=example,dc=com")
	viper.SetDefault("Monitoring.ListenAddress", ":2112")
	viper.SetDefault("Srt.ListenAddress", ":9710")
	viper.SetDefault("Srt.MaxClients", 64)
	viper.SetDefault("Web.ListenAddress", ":8080")
	viper.SetDefault("Web.Name", "Ghostream")
	viper.SetDefault("Web.Hostname", "localhost")
	viper.SetDefault("Web.Favicon", "/static/img/favicon.svg")
	viper.SetDefault("Web.ViewersCounterRefreshPeriod", 20000)
	viper.SetDefault("WebRTC.MinPortUDP", 10000)
	viper.SetDefault("WebRTC.MaxPortUDP", 10005)
	viper.SetDefault("WebRTC.STUNServers", []string{"stun:stun.l.google.com:19302"})
	viper.SetDefault("Forwarding", make(map[string][]string))

	// Copy STUN configuration to clients
	viper.Set("Web.STUNServers", viper.Get("WebRTC.STUNServers"))

	// Copy SRT server port to display it on web page
	hostport := viper.GetString("Srt.ListenAddress")
	_, srtPort, err := net.SplitHostPort(hostport)
	if err != nil {
		log.Fatalf("Failed to split host and port from %s", hostport)
	}
	viper.Set("Web.SRTServerPort", srtPort)
}

func main() {
	// Load configuration
	loadConfiguration()
	cfg := struct {
		Auth       auth.Options
		Forwarding forwarding.Options
		Monitoring monitoring.Options
		Srt        srt.Options
		Web        web.Options
		WebRTC     webrtc.Options
	}{}
	if err := viper.Unmarshal(&cfg); err != nil {
		log.Fatalln("Failed to load settings", err)
	}

	// Init authentification
	authBackend, err := auth.New(&cfg.Auth)
	if err != nil {
		log.Fatalln("Failed to load authentification backend:", err)
	}
	defer authBackend.Close()

	// WebRTC session description channels
	remoteSdpChan := make(chan webrtc.SessionDescription)
	localSdpChan := make(chan webrtc.SessionDescription)

	// SRT channel, to propagate forwarding
	forwardingChannel := make(chan srt.Packet, 65536)

	// Start stream, web and monitoring server, and stream forwarding
	go forwarding.Serve(cfg.Forwarding, forwardingChannel)
	go monitoring.Serve(&cfg.Monitoring)
	go srt.Serve(&cfg.Srt, authBackend, forwardingChannel)
	go web.Serve(remoteSdpChan, localSdpChan, &cfg.Web)
	go webrtc.Serve(remoteSdpChan, localSdpChan, &cfg.WebRTC)

	// Wait for routines
	select {}
}
