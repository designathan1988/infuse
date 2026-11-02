package api

// appLocalSettings: the local layer as the settings screen sees it. A variable's value never leaves the
// server: the screen gets the mask, sends it back for what it did not change, and the file keeps the value.

import (
	"encoding/json"
	"errors"
	"infuse/internal/config"
	"infuse/internal/core"
	"net/http"
)

// maskedLayers is config.SettingsLayers with every variable's value replaced by the mask.
func maskedLayers(root string) []config.SettingsLayer {
	layers := config.SettingsLayers(root)
	for i := range layers {
		env := map[string]string{}
		for k := range layers[i].Settings.Env {
			env[k] = keyMask
		}
		layers[i].Settings.Env = env
	}
	return layers
}

func storedLocalEnv(root string) map[string]string {
	for _, l := range config.SettingsLayers(root) {
		if l.Name == "local" {
			return l.Settings.Env
		}
	}
	return nil
}

// restoreMaskedEnv puts the stored value back under each mask; a mask with no stored value is dropped.
func restoreMaskedEnv(env, stored map[string]string) {
	for k, v := range env {
		if v != keyMask {
			continue
		}
		if old, ok := stored[k]; ok {
			env[k] = old
		} else {
			delete(env, k)
		}
	}
}

// reservedEnvError refuses out loud what the merge would silently ignore.
func reservedEnvError(root string, env map[string]string) error {
	for k := range env {
		if config.ReservedEnv(k) {
			return errors.New(core.M("settings.env_reserved", "name", k).Render(config.RootLanguage(root)))
		}
	}
	return nil
}

func saveLocalSettings(w http.ResponseWriter, body map[string]any, root string) {
	var p config.ProjectSettings
	b, err := json.Marshal(body["settings"])
	if err == nil {
		err = json.Unmarshal(b, &p)
	}
	if err == nil {
		err = reservedEnvError(root, p.Env)
	}
	if err == nil {
		restoreMaskedEnv(p.Env, storedLocalEnv(root))
		err = config.SaveLocalSettings(root, p)
	}
	sendJSON(w, 200, map[string]any{"ok": err == nil, "error": errString(err)})
}
