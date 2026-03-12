package utils

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/adrg/xdg"
	"github.com/denisbrodbeck/machineid"
	"github.com/google/uuid"
)

// XDGLogFile returns the path for a log file in the state directory, creating it if needed.
func XDGLogFile(filename string) (string, error) {
	return xdg.StateFile(filepath.Join("dreamfs", "logs", filename))
}

// GetWorkerLogger returns a logger that writes to a worker-specific file in XDG State.
func GetWorkerLogger(workerIndex string) (*log.Logger, *os.File, error) {
	// Subordinate ID: UUID:WORKER_ID
	workerID := fmt.Sprintf("%s:%s", HostID, workerIndex)
	safeID := strings.ReplaceAll(workerID, ":", "_") // Safe for filename
	logPath, err := XDGLogFile(fmt.Sprintf("worker-%s.log", safeID))
	if err != nil {
		return nil, nil, err
	}
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, nil, err
	}
	logger := log.New(f, fmt.Sprintf("[%s] ", workerID), log.LstdFlags)
	return logger, f, nil
}

// DefaultBoltDBPath returns the system-appropriate default DB path.
func DefaultBoltDBPath() string {
	// Ensure the parent directory exists and return the full path.
	dir := filepath.Join(xdg.DataHome, "dreamfs")
	os.MkdirAll(dir, 0755)
	return filepath.Join(dir, "dreamfs.db")
}

// XDGDataHome returns the XDG data home directory.
func XDGDataHome() string {
	return xdg.DataHome
}

// XDGConfigHome returns the XDG config home directory.
func XDGConfigHome() string {
	return xdg.ConfigHome
}

// XDGStateHome returns the XDG state home directory.
func XDGStateHome() string {
	return xdg.StateHome
}

// XDGConfigFile returns the path for a config file, creating directories if needed.
func XDGConfigFile(filename string) string {
	dir := filepath.Join(xdg.ConfigHome, "dreamfs")
	os.MkdirAll(dir, 0755)
	return filepath.Join(dir, filename)
}

// XDGStateFile returns the path for a state file, creating directories if needed.
func XDGStateFile(filename string) string {
	dir := filepath.Join(xdg.StateHome, "dreamfs")
	os.MkdirAll(dir, 0755)
	return filepath.Join(dir, filename)
}

var HostID string

// SetHostID allows the value to be overridden by config value
func SetHostID(cfgHost ...string) {
	// if a string was provided, use that
	if len(cfgHost) == 1 {
		HostID = cfgHost[0]
	} else {
		// otherwise we'll use the machineid library
		id, err := machineid.ProtectedID("DreamFS")
		if err != nil {
			log.Fatal(err)
		}
		HostID = id
	}
}

// GenerateUUID generates a 'v5 UUID' for a string value
func GenerateUUID(data string) string {
	// instantiate the UUID object and return as a string
	uuid := uuid.NewSHA1(uuid.NameSpaceURL, []byte(data))
	return uuid.String()
}

// ShortenString uses base64 encoding to shorten a string
func ShortenString(data string) string {
	// URL safe encoding; should be 22 chars in length
	return base64.RawURLEncoding.EncodeToString([]byte(data))
}

// PrintJSON pretty-prints any interface as JSON to stdout.
func PrintJSON(v interface{}) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Printf("error marshaling json: %v\n", err)
		return
	}
	fmt.Println(string(b))
}
