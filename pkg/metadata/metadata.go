package metadata

import (
	"encoding/json"
	"fmt"
)

type FileMetadata struct {
        ID       string                 `json:"_id"`
        FilePath string                 `json:"filePath"`
        Size     int64                  `json:"size"`
        ModTime  string                 `json:"modTime"`
        Extra    map[string]interface{} `json:"-"`
}

func (fm *FileMetadata) UnmarshalJSON(data []byte) error {
        var tmp map[string]interface{}
        if err := json.Unmarshal(data, &tmp); err != nil {
                return err
        }
        if id, ok := tmp["_id"].(string); ok {
                fm.ID = id
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
        delete(tmp, "_id")
        delete(tmp, "filePath")
        delete(tmp, "size")
        delete(tmp, "modTime")
        fm.Extra = tmp
        return nil
}

func (fm *FileMetadata) MarshalJSON() ([]byte, error) {
        m := map[string]interface{}{
                "_id":      fm.ID,
                "filePath": fm.FilePath,
                "size":     fm.Size,
                "modTime":  fm.ModTime,
        }
        for k, v := range fm.Extra {
                if _, exists := m[k]; !exists {
                        m[k] = v
                }
        }
        return json.Marshal(m)
}