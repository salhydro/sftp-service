package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"sftp-service/internal/auth"
	"sftp-service/internal/config"
	"sftp-service/internal/proxy"
	"sftp-service/internal/sftp"
)

func main() {
	log.Println("Starting SFTP service...")

	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize web API authenticator
	authenticator := auth.NewWebAPIAuthenticator(cfg.FuturAPIURL)

	// Create SFTP server (storage instances will be created per user session)
	sftpServer, err := sftp.NewServer(&sftp.Config{
		Authenticator: authenticator,
		BaseURL:       cfg.FuturAPIURL,
		HostKeyPath:   cfg.SFTPHostKeyPath,
		Port:          cfg.SFTPPort,
	})
	if err != nil {
		log.Fatalf("Failed to create SFTP server: %v", err)
	}

	// Set up graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	// Start SFTP server in a goroutine
	go func() {
		if err := sftpServer.Start(); err != nil {
			log.Fatalf("SFTP server error: %v", err)
		}
	}()

	// Start HTTP proxy in a goroutine
	go func() {
		if err := proxy.Start(&proxy.Config{
			FuturAPIURL: cfg.FuturAPIURL,
			Port:        cfg.HTTPProxyPort,
		}); err != nil {
			log.Fatalf("HTTP proxy error: %v", err)
		}
	}()

	// Wait for shutdown signal
	<-c
	log.Println("Shutting down SFTP service...")
}
