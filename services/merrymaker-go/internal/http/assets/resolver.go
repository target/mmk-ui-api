package assets

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const defaultDevReloadInterval = 50 * time.Millisecond

// AssetResolver resolves logical asset names to hashed filenames using manifest.json.
type AssetResolver struct {
	manifest          map[string]string
	mu                sync.RWMutex
	manifestPath      string
	diskPath          string
	fsys              fs.FS
	lastModTime       time.Time
	lastDevReload     time.Time
	devReloadInterval time.Duration
	logger            *slog.Logger
}

// NewAssetResolverFromDisk creates an asset resolver that reads the manifest from the local filesystem.
func NewAssetResolverFromDisk(manifestPath string) (*AssetResolver, error) {
	resolver := &AssetResolver{
		manifest:     make(map[string]string),
		manifestPath: manifestPath,
		diskPath:     manifestPath,
		logger:       slog.Default(),
	}
	return resolver, resolver.Reload()
}

// NewAssetResolverFromFS creates an asset resolver that reads the manifest from an fs.FS implementation.
func NewAssetResolverFromFS(fsys fs.FS, manifestPath string) (*AssetResolver, error) {
	resolver := &AssetResolver{
		manifest:     make(map[string]string),
		manifestPath: manifestPath,
		fsys:         fsys,
		logger:       slog.Default(),
	}
	return resolver, resolver.Reload()
}

// Reload synchronizes the in-memory manifest with the on-disk manifest.json.
func (ar *AssetResolver) Reload() error {
	ar.mu.Lock()
	defer ar.mu.Unlock()
	return ar.loadManifestLocked()
}

// ReloadIfChanged reloads the manifest if the underlying file has been updated.
func (ar *AssetResolver) ReloadIfChanged() {
	if ar == nil || ar.diskPath == "" {
		return
	}

	info, err := os.Stat(ar.diskPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			ar.mu.Lock()
			ar.manifest = make(map[string]string)
			ar.lastModTime = time.Time{}
			ar.mu.Unlock()
		}
		return
	}

	ar.mu.RLock()
	last := ar.lastModTime
	ar.mu.RUnlock()

	if !info.ModTime().After(last) {
		return
	}

	if reloadErr := ar.Reload(); reloadErr != nil {
		ar.loggerOrDefault().Error("failed to reload asset manifest",
			slog.String("manifest", ar.manifestPath),
			slog.Any("error", reloadErr),
		)
	}
}

// Resolve returns the hashed filename for a logical asset name.
func (ar *AssetResolver) Resolve(logicalName string) string {
	ar.mu.RLock()
	defer ar.mu.RUnlock()

	if hashedName, exists := ar.manifest[logicalName]; exists {
		return "/static/" + hashedName
	}

	// Fallback to logical name if not in manifest (dev mode)
	return "/static/" + logicalName
}

// ManifestPath returns the manifest path configured for the resolver.
func (ar *AssetResolver) ManifestPath() string {
	if ar == nil {
		return ""
	}
	return ar.manifestPath
}

// loadManifestLocked reads the manifest.json file and populates the asset mapping.
// Callers must hold ar.mu.Lock().
func (ar *AssetResolver) loadManifestLocked() error {
	if ar.manifest == nil {
		ar.manifest = make(map[string]string)
	}
	if ar.manifestPath == "" && ar.diskPath == "" && ar.fsys == nil {
		ar.resetManifest()
		return nil
	}

	data, err := ar.readManifestData()
	if err != nil {
		return err
	}

	ar.parseManifestData(data)
	ar.updateLastModTime()
	return nil
}

// resetManifest clears the manifest and last mod time.
func (ar *AssetResolver) resetManifest() {
	ar.manifest = make(map[string]string)
	ar.lastModTime = time.Time{}
}

// readManifestData reads the manifest data from disk or embedded FS.
func (ar *AssetResolver) readManifestData() ([]byte, error) {
	switch {
	case ar.diskPath != "":
		return ar.readFromDisk()
	case ar.fsys != nil:
		return ar.readFromFS()
	default:
		return nil, errors.New("no manifest source configured")
	}
}

// readFromDisk reads the manifest from disk.
func (ar *AssetResolver) readFromDisk() ([]byte, error) {
	data, err := os.ReadFile(ar.diskPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			ar.resetManifest()
			return nil, nil
		}
		return nil, err
	}
	return data, nil
}

// readFromFS reads the manifest from embedded FS.
func (ar *AssetResolver) readFromFS() ([]byte, error) {
	data, err := fs.ReadFile(ar.fsys, ar.manifestPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			ar.resetManifest()
			return nil, nil
		}
		return nil, err
	}
	return data, nil
}

// parseManifestData parses the manifest JSON data.
func (ar *AssetResolver) parseManifestData(data []byte) {
	if len(data) == 0 {
		ar.manifest = make(map[string]string)
		return
	}

	var manifest map[string]string
	if err := json.Unmarshal(data, &manifest); err != nil {
		ar.loggerOrDefault().Error("failed to parse asset manifest",
			slog.String("manifest", ar.manifestPath),
			slog.Any("error", err),
		)
		ar.manifest = make(map[string]string)
		return
	}
	ar.manifest = manifest
}

// updateLastModTime updates the last modification time for disk-based manifests.
func (ar *AssetResolver) updateLastModTime() {
	if ar.diskPath == "" {
		return
	}
	if info, err := os.Stat(ar.diskPath); err == nil {
		ar.lastModTime = info.ModTime()
	} else {
		ar.lastModTime = time.Time{}
	}
}

// ResolveAsset resolves a logical asset name to its physical path using the resolver and dev mode options.
func ResolveAsset(resolver *AssetResolver, logicalName string, devMode bool) string {
	defaultPath := "/static/" + logicalName

	if resolver == nil {
		return defaultPath
	}

	if devMode {
		if err := resolver.reloadForDev(); err != nil {
			resolver.loggerOrDefault().Error("failed to reload asset manifest",
				slog.String("manifest", resolver.manifestPath),
				slog.Any("error", err),
			)
		}
	} else {
		resolver.ReloadIfChanged()
	}

	resolved := resolver.Resolve(logicalName)

	if devMode {
		resolved = resolver.verifyAssetInDevMode(logicalName, resolved, defaultPath)
	}

	return resolved
}

func (ar *AssetResolver) verifyAssetInDevMode(logicalName, resolved, defaultPath string) string {
	if AssetExistsOnDisk(resolved) {
		return resolved
	}

	// Asset missing, try reloading manifest
	if err := ar.Reload(); err != nil {
		ar.loggerOrDefault().Error("failed to reload asset manifest after missing asset",
			slog.String("manifest", ar.manifestPath),
			slog.Any("error", err),
			slog.String("logical_asset", logicalName),
		)
	}

	resolved = ar.Resolve(logicalName)
	if !AssetExistsOnDisk(resolved) {
		ar.loggerOrDefault().Warn("resolved asset missing on disk; using logical name",
			slog.String("logical_asset", logicalName),
			slog.String("resolved_asset", resolved),
		)
		return defaultPath
	}

	return resolved
}

// AssetExistsOnDisk reports whether the resolved asset exists on disk in developer mode.
func AssetExistsOnDisk(resolvedPath string) bool {
	const prefix = "/static/"
	if !strings.HasPrefix(resolvedPath, prefix) {
		return false
	}

	rel := strings.TrimPrefix(resolvedPath, prefix)
	if rel == "" {
		return false
	}

	searchPaths := []string{
		filepath.Join("frontend", "static", rel),
		filepath.Join("frontend", "public", rel),
	}

	for _, p := range searchPaths {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}

// SetLogger updates the resolver's logger. If logger is nil, slog.Default() is used.
func (ar *AssetResolver) SetLogger(logger *slog.Logger) {
	ar.mu.Lock()
	defer ar.mu.Unlock()
	if logger == nil {
		ar.logger = slog.Default()
		return
	}
	ar.logger = logger
}

// SetDevReloadInterval overrides the minimum interval between reload attempts in dev mode.
func (ar *AssetResolver) SetDevReloadInterval(interval time.Duration) {
	ar.mu.Lock()
	defer ar.mu.Unlock()
	ar.devReloadInterval = interval
}

func (ar *AssetResolver) reloadForDev() error {
	if ar == nil {
		return nil
	}

	ar.mu.RLock()
	interval := ar.devReloadInterval
	ar.mu.RUnlock()
	if interval <= 0 {
		interval = defaultDevReloadInterval
	}

	now := time.Now()

	ar.mu.Lock()
	last := ar.lastDevReload
	if !last.IsZero() && now.Sub(last) < interval {
		ar.mu.Unlock()
		return nil
	}
	ar.lastDevReload = now
	ar.mu.Unlock()

	if err := ar.Reload(); err != nil {
		ar.mu.Lock()
		if ar.lastDevReload.Equal(now) {
			ar.lastDevReload = time.Time{}
		}
		ar.mu.Unlock()
		return err
	}

	return nil
}

func (ar *AssetResolver) loggerOrDefault() *slog.Logger {
	if ar != nil && ar.logger != nil {
		return ar.logger
	}
	return slog.Default()
}
