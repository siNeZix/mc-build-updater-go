package remote

// Mod is deliberately compatible with legacy /MM/<branch>.json records.
// Path holds uploader's original local path and lets client derive filename.
type Mod struct {
	Hash string `json:"hash"`
	Path string `json:"path"`
}
