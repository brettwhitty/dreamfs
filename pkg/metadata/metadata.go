package metadata

import (
	"encoding/json"
	"fmt"
)

type FileMetadata struct {
	ID       string                 `json:"_id"`      // Unique document ID (the fingerprint)
	IDString string                 `json:"idString"` // Composite string used to generate ID
	HostID   string                 `json:"hostID"`   // ID of the host where the file was indexed
	FilePath string                 `json:"filePath"`
	Size     int64                  `json:"size"`
	ModTime  string                 `json:"modTime"`
	BLAKE3   string                 `json:"blake3"` // BLAKE3 hash of the file content
	Extra    map[string]interface{} `json:"-"`
}

func (fm *FileMetadata) UnmarshalJSON(data []byte) error {
	var tmp map[string]interface{}
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}

	// Unmarshal known fields
	if id, ok := tmp["_id"].(string); ok {
		fm.ID = id
	}
	if idString, ok := tmp["idString"].(string); ok {
		fm.IDString = idString
	}
	if hostID, ok := tmp["hostID"].(string); ok {
		fm.HostID = hostID
	}
	if fp, ok := tmp["filePath"].(string); ok {
		fm.FilePath = fp
	}
	if size, ok := tmp["size"].(float64); ok {
		fm.Size = int64(size)
	}
	if mt, ok := tmp["modTime"].(string); ok {
		fm.ModTime = mt
	}
	if blake3, ok := tmp["blake3"].(string); ok {
		fm.BLAKE3 = blake3
	}

	// Populate Extra map with unknown fields
	fm.Extra = make(map[string]interface{})
	for k, v := range tmp {
		switch k {
		case "_id", "idString", "hostID", "filePath", "size", "modTime", "blake3":
			// Skip known fields
		default:
			fm.Extra[k] = v
		}
	}
	return nil
}

func (fm *FileMetadata) MarshalJSON() ([]byte, error) {
	m := map[string]interface{}{
		"_id":      fm.ID,
		"idString": fm.IDString,
		"hostID":   fm.HostID,
		"filePath": fm.FilePath,
		"size":     fm.Size,
		"modTime":  fm.ModTime,
		"blake3":   fm.BLAKE3,
	}
	for k, v := range fm.Extra {
		if _, exists := m[k]; !exists { // Only add if not a known field
			m[k] = v
		}
	}
	return json.Marshal(m)
}
