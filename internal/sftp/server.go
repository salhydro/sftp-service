package sftp

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net"
	"os"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"sftp-service/internal/auth"
)

type Server struct {
	authenticator *auth.WebAPIAuthenticator
	baseURL       string
	hostKey       ssh.Signer
	port          string
}

type Config struct {
	Authenticator *auth.WebAPIAuthenticator
	BaseURL       string
	HostKeyPath   string
	Port          string
}

// NewServer creates a new SFTP server
func NewServer(config *Config) (*Server, error) {
	hostKey, err := loadOrCreateHostKey(config.HostKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load host key: %w", err)
	}

	return &Server{
		authenticator: config.Authenticator,
		baseURL:       config.BaseURL,
		hostKey:       hostKey,
		port:          config.Port,
	}, nil
}

// Start starts the SFTP server
func (s *Server) Start() error {
	log.Printf("Starting SFTP server on port %s", s.port)

	// Configure SSH server
	sshConfig := &ssh.ServerConfig{
		PasswordCallback: s.passwordCallback,
		MaxAuthTries:     3,
	}
	sshConfig.AddHostKey(s.hostKey)

	// Listen for connections
	listener, err := net.Listen("tcp", ":"+s.port)
	if err != nil {
		return fmt.Errorf("failed to listen on port %s: %w", s.port, err)
	}
	defer listener.Close()

	log.Printf("SFTP server listening on port %s", s.port)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Failed to accept connection: %v", err)
			continue
		}

		// Handle connection in a goroutine
		go s.handleConnection(conn, sshConfig)
	}
}

func (s *Server) passwordCallback(conn ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
	username := conn.User()
	log.Printf("Authentication attempt for user: %s", username)

	user, err := s.authenticator.AuthenticateUser(username, string(password))
	if err != nil {
		log.Printf("Authentication failed for user %s: %v", username, err)
		return nil, fmt.Errorf("authentication failed")
	}

	log.Printf("Authentication successful for user: %s", username)

	// Store username, user ID and API key in permissions for later use
	return &ssh.Permissions{
		Extensions: map[string]string{
			"username": user.Username,
			"user_id":  user.ID,
			"api_key":  user.ApiKey,
		},
	}, nil
}

func (s *Server) handleConnection(conn net.Conn, sshConfig *ssh.ServerConfig) {
	defer conn.Close()

	remoteAddr := conn.RemoteAddr().String()
	log.Printf("New TCP connection from %s", remoteAddr)

	// Perform SSH handshake
	sshConn, chans, reqs, err := ssh.NewServerConn(conn, sshConfig)
	if err != nil {
		// EOF typically means the client (or NLB health check) closed the connection
		// without completing the SSH handshake — log at lower verbosity
		if err.Error() == "EOF" {
			log.Printf("Connection from %s closed before SSH handshake (likely NLB health check)", remoteAddr)
		} else {
			log.Printf("SSH handshake failed from %s: %v", remoteAddr, err)
		}
		return
	}
	defer sshConn.Close()

	// Get username and API key from permissions
	username := sshConn.Permissions.Extensions["username"]
	apiKey := sshConn.Permissions.Extensions["api_key"]
	log.Printf("SSH connection established from %s for user %s (session: %x)", remoteAddr, username, sshConn.SessionID())

	// Handle global requests
	go ssh.DiscardRequests(reqs)

	// Handle channels
	for newChannel := range chans {
		if newChannel.ChannelType() != "session" {
			newChannel.Reject(ssh.UnknownChannelType, "unknown channel type")
			continue
		}

		channel, requests, err := newChannel.Accept()
		if err != nil {
			log.Printf("Failed to accept channel: %v", err)
			continue
		}

		// Handle channel requests
		go func(in <-chan *ssh.Request) {
			for req := range in {
				switch req.Type {
				case "subsystem":
					if string(req.Payload[4:]) == "sftp" {
						req.Reply(true, nil)
						s.handleSFTP(channel, username, apiKey)
					} else {
						req.Reply(false, nil)
					}
				default:
					req.Reply(false, nil)
				}
			}
		}(requests)
	}
}

func (s *Server) handleSFTP(channel ssh.Channel, username, apiKey string) {
	defer channel.Close()

	log.Printf("Starting SFTP session for user: %s", username)

	// Create API-backed file system for the user
	filesystem := NewAPIFileSystem(s.baseURL, username, apiKey)

	// Create handlers
	handlers := sftp.Handlers{
		FileGet:  filesystem,
		FilePut:  filesystem,
		FileCmd:  filesystem,
		FileList: filesystem,
	}

	// Create SFTP request server
	requestServer := sftp.NewRequestServer(channel, handlers)

	// Serve SFTP requests
	if err := requestServer.Serve(); err != nil && err != io.EOF {
		log.Printf("SFTP server error: %v", err)
	}

	log.Printf("SFTP session ended for user: %s", username)
}

// loadOrCreateHostKey loads an existing host key or creates a new one
func loadOrCreateHostKey(hostKeyPath string) (ssh.Signer, error) {
	// Try to load existing key
	if _, err := os.Stat(hostKeyPath); err == nil {
		keyData, err := os.ReadFile(hostKeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read host key: %w", err)
		}

		key, err := ssh.ParsePrivateKey(keyData)
		if err != nil {
			return nil, fmt.Errorf("failed to parse host key: %w", err)
		}

		log.Printf("Loaded existing host key from %s", hostKeyPath)
		return key, nil
	}

	// Create new key
	log.Printf("Creating new host key at %s", hostKeyPath)

	privateKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, fmt.Errorf("failed to generate RSA key: %w", err)
	}

	privateKeyBytes := x509.MarshalPKCS1PrivateKey(privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal private key: %w", err)
	}

	privateKeyPEM := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privateKeyBytes,
	}

	keyFile, err := os.Create(hostKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create host key file: %w", err)
	}
	defer keyFile.Close()

	if err := pem.Encode(keyFile, privateKeyPEM); err != nil {
		return nil, fmt.Errorf("failed to write host key: %w", err)
	}

	// Set proper permissions
	if err := os.Chmod(hostKeyPath, 0600); err != nil {
		return nil, fmt.Errorf("failed to set host key permissions: %w", err)
	}

	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create signer from key: %w", err)
	}

	return signer, nil
}
