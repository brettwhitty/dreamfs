package utils

import (
	"path/filepath"

	"encoding/base64"
	"github.com/adrg/xdg"
	"github.com/denisbrodbeck/machineid"
	"github.com/google/uuid"
	"log"
)

// DefaultBoltDBPath returns the system-appropriate default DB path.
func DefaultBoltDBPath() string {
	// Use XDG data home; for Windows or macOS this resolves appropriately.
	dataHome := xdg.DataHome
	return filepath.Join(dataHome, "indexer", "indexer.db")
}

// XDGDataHome returns the XDG data home directory.
func XDGDataHome() string {
	return xdg.DataHome
}

var HostID string

// SetHostID allows the value to be overridden by config value
func SetHostID(cfgHost ...string) {
	// if a string was provided, use that
	if (len(cfgHost) == 1) {
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