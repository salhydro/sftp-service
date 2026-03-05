package sftp

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"sftp-service/internal/storage"

	"github.com/pkg/sftp"
)

// APIFileSystem implements sftp.FileLister, sftp.FileReader, sftp.FileWriter, sftp.FileCmder, and sftp.FileStater interfaces
type APIFileSystem struct {
	apiURL      string // API base URL for both pricelist and incoming orders
	username    string
	apiKey      string   // API key for authenticated calls
	allowedDirs []string // Allowed directories for this user
	allowedOps  []string // Allowed operations
}

// NewAPIFileSystem creates a new API-backed file system with restricted access
func NewAPIFileSystem(apiURL, username, apiKey string) *APIFileSystem {
	return &APIFileSystem{
		apiURL:      apiURL,
		username:    username,
		apiKey:      apiKey,
		allowedDirs: []string{"/", "/in", "/Hinnat"},           // Only root, in, and Hinnat directories
		allowedOps:  []string{"list", "read", "write-in-only"}, // List and read everywhere, write only to /in
	}
}

// isPathAllowed checks if the given path is allowed for the user
func (fs *APIFileSystem) isPathAllowed(path string) bool {
	// Normalize path
	if path == "" || path == "." {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	// Check if path starts with any allowed directory
	for _, allowedDir := range fs.allowedDirs {
		if path == allowedDir || strings.HasPrefix(path, allowedDir+"/") {
			return true
		}
	}

	return false
} // isWriteAllowed checks if writing is allowed in the given path
func (fs *APIFileSystem) isWriteAllowed(path string) bool {
	// Only allow writing to /in directory (not root or Hinnat)
	if path == "/" {
		return false
	}

	return strings.HasPrefix(path, "/in/") || path == "/in"
}

// isInIncomingDirectory checks if path is in /in/ directory
func (fs *APIFileSystem) isInIncomingDirectory(path string) bool {
	return strings.HasPrefix(path, "/in/") && !strings.Contains(strings.TrimPrefix(path, "/in/"), "/")
}

// Realpath resolves absolute paths for SFTP operations
func (fs *APIFileSystem) Realpath(path string) string {
	log.Printf("Realpath: %s", path)

	// Normalize path
	if path == "" || path == "." {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	log.Printf("Realpath resolved to: '%s'", path)
	return path
}

// Fileread implements sftp.FileReader
func (fs *APIFileSystem) Fileread(r *sftp.Request) (io.ReaderAt, error) {
	log.Printf("Reading file: %s (user: %s)", r.Filepath, fs.username)

	// Check if path is allowed
	if !fs.isPathAllowed(r.Filepath) {
		log.Printf("Access denied: user %s tried to read %s", fs.username, r.Filepath)
		return nil, fmt.Errorf("access denied: path not allowed")
	}

	// Deny reading from /in/ directory (write-only)
	if fs.isInIncomingDirectory(r.Filepath) {
		log.Printf("Read denied from /in/: user %s tried to read %s", fs.username, r.Filepath)
		return nil, fmt.Errorf("access denied: /in/ directory is write-only")
	}

	data, err := storage.DownloadPricelist(fs.apiURL, fs.username, fs.apiKey, r.Filepath)
	if err != nil {
		return nil, err
	}

	return &bytesReaderAt{data: data}, nil
}

// Filewrite implements sftp.FileWriter
func (fs *APIFileSystem) Filewrite(r *sftp.Request) (io.WriterAt, error) {
	log.Printf("SFTP Write %s: %s (user: %s)", r.Method, r.Filepath, fs.username)

	// Check if path is allowed for writing
	if !fs.isPathAllowed(r.Filepath) || !fs.isWriteAllowed(r.Filepath) {
		log.Printf("Write access denied: user %s tried to write to %s", fs.username, r.Filepath)
		return nil, fmt.Errorf("access denied: write not allowed to this path")
	}

	// Handle /in/ directory separately (file storage)
	if fs.isInIncomingDirectory(r.Filepath) {
		filename := filepath.Base(r.Filepath)
		return &incomingWriterAt{
			apiURL:   fs.apiURL,
			username: fs.username,
			apiKey:   fs.apiKey,
			filename: filename,
		}, nil
	}

	// Handle /Hinnat/ directory (read-only, no writes allowed)
	return nil, fmt.Errorf("access denied: /Hinnat directory is read-only")
}

// Filecmd implements sftp.FileCmder
func (fs *APIFileSystem) Filecmd(r *sftp.Request) error {
	log.Printf("SFTP command: %s %s (user: %s)", r.Method, r.Filepath, fs.username)

	// Check if path is allowed
	if !fs.isPathAllowed(r.Filepath) {
		log.Printf("Command access denied: user %s tried %s on %s", fs.username, r.Method, r.Filepath)
		return fmt.Errorf("access denied: path not allowed")
	}

	switch r.Method {
	case "Remove":
		// Deny all delete operations
		log.Printf("Delete denied: user %s tried to delete %s", fs.username, r.Filepath)
		return fmt.Errorf("access denied: delete operations not allowed")
	case "Mkdir":
		log.Printf("Mkdir denied: user %s tried to create directory %s", fs.username, r.Filepath)
		return fmt.Errorf("access denied: mkdir operations not allowed")
	case "Rename":
		// Deny all rename operations
		log.Printf("Rename denied: user %s tried to rename %s", fs.username, r.Filepath)
		return fmt.Errorf("access denied: rename operations not allowed")
	case "Rmdir":
		// Deny all directory removal operations
		log.Printf("Rmdir denied: user %s tried to remove directory %s", fs.username, r.Filepath)
		return fmt.Errorf("access denied: directory removal not allowed")
	default:
		return sftp.ErrSSHFxOpUnsupported
	}
}

// Filelist implements sftp.FileLister
func (fs *APIFileSystem) Filelist(r *sftp.Request) (sftp.ListerAt, error) {
	log.Printf("SFTP %s: %s (user: %s)", r.Method, r.Filepath, fs.username)

	// Log warning for unsupported Readlink operations
	if r.Method == "Readlink" {
		log.Printf("WARNING: FileLister method 'Readlink' not supported for %s (symbolic links not supported)", r.Filepath)
		return nil, sftp.ErrSSHFxOpUnsupported
	}

	// Check if path is allowed
	if !fs.isPathAllowed(r.Filepath) {
		log.Printf("Access denied: user %s tried %s on %s", fs.username, r.Method, r.Filepath)
		return nil, fmt.Errorf("access denied: path not allowed")
	}

	// Handle root directory specially - show only allowed subdirectories
	if r.Filepath == "/" || r.Filepath == "" {
		return fs.listRootDirectory()
	}

	// Handle /in/ directory specially (PostgreSQL storage)
	if r.Filepath == "/in" {
		if r.Method == "Stat" {
			// Return directory info for stat request (cd command)
			return fs.statInDirectory()
		} else {
			// List files inside directory for ls command
			return fs.listInDirectory()
		}
	}

	// Handle /Hinnat directory
	if r.Filepath == "/Hinnat" {
		if r.Method == "Stat" {
			// Return directory info for stat request (cd command)
			return fs.statHinnatDirectory()
		} else {
			// List files inside directory for ls command
			return fs.listHinnatDirectory()
		}
	}

	// Handle /Hinnat/salhydro_kaikki.zip file specifically
	if r.Filepath == "/Hinnat/salhydro_kaikki.zip" {
		fileInfo := &apiFileInfo{
			name:    "salhydro_kaikki.zip",
			size:    2 * 1024 * 1024,
			modTime: time.Now(),
			isDir:   false,
		}
		return &listerat{files: []os.FileInfo{fileInfo}}, nil
	}

	// If no specific handler found, return empty file list
	var fileInfos []os.FileInfo
	return &listerat{files: fileInfos}, nil
}

// listHinnatDirectory returns files inside /Hinnat directory
func (fs *APIFileSystem) listHinnatDirectory() (sftp.ListerAt, error) {
	var fileInfos []os.FileInfo
	fileInfos = append(fileInfos, &apiFileInfo{
		name:    "salhydro_kaikki.zip",
		size:    2 * 1024 * 1024, // 2MB
		modTime: time.Now(),
		isDir:   false,
	})

	return &listerat{files: fileInfos}, nil
}

// statHinnatDirectory returns directory info for /Hinnat (for cd command)
func (fs *APIFileSystem) statHinnatDirectory() (sftp.ListerAt, error) {
	fileInfo := &apiFileInfo{
		name:    "Hinnat",
		size:    0,
		modTime: time.Now(),
		isDir:   true,
	}

	return &listerat{files: []os.FileInfo{fileInfo}}, nil
}

// listInDirectory returns an empty directory (files are processed immediately on upload)
func (fs *APIFileSystem) listInDirectory() (sftp.ListerAt, error) {
	// Return empty directory - files are sent to API immediately when uploaded
	var fileInfos []os.FileInfo
	return &listerat{files: fileInfos}, nil
}

// statInDirectory returns directory info for /in (for cd command)
func (fs *APIFileSystem) statInDirectory() (sftp.ListerAt, error) {
	fileInfo := &apiFileInfo{
		name:    "in",
		size:    0,
		modTime: time.Now(),
		isDir:   true,
	}

	return &listerat{files: []os.FileInfo{fileInfo}}, nil
}

// listRootDirectory returns only the allowed directories in root
func (fs *APIFileSystem) listRootDirectory() (sftp.ListerAt, error) {
	var fileInfos []os.FileInfo

	// Add the allowed directories
	fileInfos = append(fileInfos, &apiFileInfo{
		name:    "in",
		size:    0,
		modTime: time.Now(),
		isDir:   true,
	})

	fileInfos = append(fileInfos, &apiFileInfo{
		name:    "Hinnat",
		size:    0,
		modTime: time.Now(),
		isDir:   true,
	})

	return &listerat{files: fileInfos}, nil
}

// bytesReaderAt implements io.ReaderAt for byte slices
type bytesReaderAt struct {
	data []byte
}

func (r *bytesReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if off >= int64(len(r.data)) {
		return 0, io.EOF
	}

	n := copy(p, r.data[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

// incomingWriterAt implements io.WriterAt for API /in/ directory
type incomingWriterAt struct {
	apiURL   string
	username string
	apiKey   string
	filename string
	data     []byte
}

const maxUploadSize = 102400 // 100KB upload limit

func (w *incomingWriterAt) WriteAt(p []byte, off int64) (int, error) {
	// Check upload size limit early to prevent memory exhaustion
	needed := int(off) + len(p)
	if needed > maxUploadSize {
		return 0, fmt.Errorf("file size exceeds 100KB limit")
	}

	// Extend data slice if necessary
	if needed > len(w.data) {
		newData := make([]byte, needed)
		copy(newData, w.data)
		w.data = newData
	}

	copy(w.data[off:], p)
	return len(p), nil
}

func (w *incomingWriterAt) Close() error {
	if len(w.data) > 0 {
		content := string(w.data)
		return storage.SendOrderToAPI(w.apiURL, w.username, w.apiKey, w.filename, content)
	}
	return nil
}

// apiFileInfo implements os.FileInfo for API files
type apiFileInfo struct {
	name    string
	size    int64
	modTime time.Time
	isDir   bool
}

func (fi *apiFileInfo) Name() string { return fi.name }
func (fi *apiFileInfo) Size() int64  { return fi.size }
func (fi *apiFileInfo) Mode() os.FileMode {
	if fi.isDir {
		return os.ModeDir | 0755
	}
	return 0644
}
func (fi *apiFileInfo) ModTime() time.Time { return fi.modTime }
func (fi *apiFileInfo) IsDir() bool        { return fi.isDir }
func (fi *apiFileInfo) Sys() interface{}   { return nil }

// listerat implements sftp.ListerAt
type listerat struct {
	files []os.FileInfo
}

func (l *listerat) ListAt(f []os.FileInfo, offset int64) (int, error) {
	if offset >= int64(len(l.files)) {
		return 0, io.EOF
	}

	n := copy(f, l.files[offset:])
	if offset+int64(n) >= int64(len(l.files)) {
		return n, io.EOF
	}
	return n, nil
}
