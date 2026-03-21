package legacy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"xmilo/sidecar-go/internal/db"
)

func RunOnce(store *db.Store, mindRoot string) error {
	done, err := store.GetFlag("legacy_import_completed")
	if err != nil {
		return err
	}
	if done == "true" {
		return nil
	}

	candidates := []string{
		filepath.Join(mindRoot, "tasks", "active.json"),
		filepath.Join(mindRoot, "tasks", "mission_queue.json"),
		filepath.Join(mindRoot, "tasks", "completed.json"),
		filepath.Join(mindRoot, "tasks", "failed.json"),
		filepath.Join(mindRoot, "memory", "knowledge", "device_capability_profile.json"),
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err != nil {
			continue
		}
		if strings.HasSuffix(path, "device_capability_profile.json") {
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			var payload struct {
				Capabilities []struct {
					Name              string `json:"name"`
					NeedsRevalidation bool   `json:"needs_revalidation"`
				} `json:"capabilities"`
			}
			if json.Unmarshal(raw, &payload) == nil {
				for _, capability := range payload.Capabilities {
					if capability.NeedsRevalidation {
						_ = store.QueueCapability(capability.Name)
					}
				}
			}
		}
	}

	return store.SetFlag("legacy_import_completed", "true")
}
