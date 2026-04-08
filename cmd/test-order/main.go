// Test script for uploading order files via SFTP
// Usage: go run test-order.go [options]
//
//	go run test-order.go -host futur.salhydro.fi -user customer_5229 -pass klsakldaklskldklasjd
//	go run test-order.go -host localhost -port 2222 -file samples/5229-2026_03_05-13_56_09.113.done
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

func main() {
	host := flag.String("host", "futur.salhydro.fi", "SFTP host")
	port := flag.Int("port", 22, "SFTP port")
	user := flag.String("user", "customer_5229", "SFTP username")
	pass := flag.String("pass", "klsakldaklskldklasjd", "SFTP password")
	file := flag.String("file", "", "Order file to upload (default: first .done file in samples/)")
	listOnly := flag.Bool("list", false, "Only list directories, don't upload")
	flag.Parse()

	// Find sample file if not specified
	sampleFile := *file
	if sampleFile == "" && !*listOnly {
		matches, _ := filepath.Glob("samples/*.done")
		if len(matches) == 0 {
			log.Fatal("No sample files found in samples/ directory")
		}
		sampleFile = matches[len(matches)-1] // Use the latest one
		fmt.Printf("Using sample file: %s\n", sampleFile)
	}

	// Connect via SSH
	config := &ssh.ClientConfig{
		User: *user,
		Auth: []ssh.AuthMethod{
			ssh.Password(*pass),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	addr := fmt.Sprintf("%s:%d", *host, *port)
	fmt.Printf("Connecting to %s as %s...\n", addr, *user)

	conn, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		log.Fatalf("SSH connection failed: %v", err)
	}
	defer conn.Close()
	fmt.Println("SSH connection established!")

	// Open SFTP session
	client, err := sftp.NewClient(conn)
	if err != nil {
		log.Fatalf("SFTP session failed: %v", err)
	}
	defer client.Close()
	fmt.Println("SFTP session opened!")

	// List root directory
	fmt.Println("\n--- Root directory (/) ---")
	entries, err := client.ReadDir("/")
	if err != nil {
		log.Fatalf("Failed to list root: %v", err)
	}
	for _, entry := range entries {
		fmt.Printf("  %s\t%s\t%d bytes\n", entry.Mode(), entry.Name(), entry.Size())
	}

	// List /in/ directory
	fmt.Println("\n--- /in/ directory ---")
	inEntries, err := client.ReadDir("/in")
	if err != nil {
		fmt.Printf("  (error: %v)\n", err)
	} else if len(inEntries) == 0 {
		fmt.Println("  (empty)")
	} else {
		for _, entry := range inEntries {
			fmt.Printf("  %s\t%s\t%d bytes\n", entry.Mode(), entry.Name(), entry.Size())
		}
	}

	// List /Hinnat/ directory
	fmt.Println("\n--- /Hinnat/ directory ---")
	hinnatEntries, err := client.ReadDir("/Hinnat")
	if err != nil {
		fmt.Printf("  (error: %v)\n", err)
	} else {
		for _, entry := range hinnatEntries {
			fmt.Printf("  %s\t%s\t%d bytes\n", entry.Mode(), entry.Name(), entry.Size())
		}
	}

	if *listOnly {
		fmt.Println("\nDone (list only mode).")
		return
	}

	// Upload order file
	fmt.Printf("\n--- Uploading %s to /in/ ---\n", filepath.Base(sampleFile))

	localData, err := os.ReadFile(sampleFile)
	if err != nil {
		log.Fatalf("Failed to read local file: %v", err)
	}
	fmt.Printf("Local file size: %d bytes\n", len(localData))

	remotePath := "/in/" + filepath.Base(sampleFile)
	remoteFile, err := client.Create(remotePath)
	if err != nil {
		log.Fatalf("Failed to create remote file: %v", err)
	}

	n, err := remoteFile.Write(localData)
	if err != nil {
		log.Fatalf("Failed to write to remote file: %v", err)
	}

	if err := remoteFile.Close(); err != nil {
		log.Fatalf("Failed to close remote file: %v", err)
	}

	fmt.Printf("Uploaded %d bytes to %s\n", n, remotePath)
	fmt.Println("\nOrder upload successful!")
}
