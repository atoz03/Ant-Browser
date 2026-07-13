package browser

type Extension struct {
	ExtensionID       string `json:"extensionId"`
	Name              string `json:"name"`
	Version           string `json:"version"`
	Description       string `json:"description"`
	IconDataURL       string `json:"iconDataUrl"`
	ManifestJSON      string `json:"manifestJson"`
	SourceURL         string `json:"sourceUrl"`
	InstallDir        string `json:"installDir"`
	Enabled           bool   `json:"enabled"`
	ScopeRestricted   bool   `json:"scopeRestricted"`
	ScopeProfileCount int    `json:"scopeProfileCount"`
	InstalledAt       string `json:"installedAt"`
	UpdatedAt         string `json:"updatedAt"`
}

type ExtensionLookupResult struct {
	ExtensionID string `json:"extensionId"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	StoreURL    string `json:"storeUrl"`
	Installable bool   `json:"installable"`
	Message     string `json:"message"`
}

type ProfileExtensionSettings struct {
	ProfileID           string   `json:"profileId"`
	Configured          bool     `json:"configured"`
	ExtensionIDs        []string `json:"extensionIds"`
	AllowedExtensionIDs []string `json:"allowedExtensionIds"`
	UpdatedAt           string   `json:"updatedAt"`
}

// ExtensionProfileScope 表示插件允许加载到哪些实例。
// Restricted=false 时默认允许所有实例；Restricted=true 时只允许 ProfileIDs 中的实例。
type ExtensionProfileScope struct {
	ExtensionID string   `json:"extensionId"`
	Restricted  bool     `json:"restricted"`
	ProfileIDs  []string `json:"profileIds"`
	UpdatedAt   string   `json:"updatedAt"`
}
