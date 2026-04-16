package storage

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

type FileStore struct {
	baseDir    string
	maxResults int
	retention  time.Duration
	mu         sync.RWMutex
	listCache  map[string]listCacheEntry
	indexCache map[string]summaryIndexCache
}

type Item struct {
	ID      string    `json:"id"`
	Path    string    `json:"path"`
	ModTime time.Time `json:"mod_time"`
	Size    int64     `json:"size"`
}

type listCacheEntry struct {
	modTime time.Time
	size    int64
	items   []Item
}

type summaryIndexCache struct {
	modTime time.Time
	size    int64
	entries map[string]json.RawMessage
}

var (
	validStoreCategories = map[string]struct{}{
		"probes":  {},
		"benches": {},
		"certs":   {},
	}
	validStoreID = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	summaryDirs  = map[string]string{
		"probes":  "probe_summaries",
		"benches": "bench_summaries",
	}
)

func NewFileStore(baseDir string, maxResults int, retention time.Duration) (*FileStore, error) {
	if err := os.MkdirAll(baseDir, 0o750); err != nil {
		return nil, err
	}
	for _, dir := range []string{"probes", "benches", "certs", "probe_summaries", "bench_summaries"} {
		if err := os.MkdirAll(filepath.Join(baseDir, dir), 0o750); err != nil {
			return nil, err
		}
	}
	return &FileStore{
		baseDir:    baseDir,
		maxResults: maxResults,
		retention:  retention,
		listCache:  map[string]listCacheEntry{},
		indexCache: map[string]summaryIndexCache{},
	}, nil
}

func (s *FileStore) SaveProbe(id string, data any) error {
	return s.save("probes", id, data)
}

func (s *FileStore) SaveBench(id string, data any) error {
	return s.save("benches", id, data)
}

func (s *FileStore) SaveProbeSummary(id string, data any) error {
	return s.saveSummary("probes", id, data)
}

func (s *FileStore) SaveBenchSummary(id string, data any) error {
	return s.saveSummary("benches", id, data)
}

func (s *FileStore) save(category, id string, data any) error {
	path, err := s.resultPath(category, id)
	if err != nil {
		return err
	}
	// #nosec G304 -- resultPath validates category/id and constrains paths to the store root.
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	zip := gzip.NewWriter(file)

	if err := json.NewEncoder(zip).Encode(data); err != nil {
		_ = zip.Close()
		_ = file.Close()
		return err
	}
	if err := zip.Close(); err != nil {
		_ = file.Close()
		return fmt.Errorf("gzip close: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("file close: %w", err)
	}
	s.invalidateListCache(category)
	return s.cleanup(category)
}

func (s *FileStore) List(category string) ([]Item, error) {
	if err := validateStoreCategory(category); err != nil {
		return nil, err
	}
	dir := filepath.Join(s.baseDir, category)
	info, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	s.mu.RLock()
	cached, ok := s.listCache[category]
	s.mu.RUnlock()
	if ok && cached.modTime.Equal(info.ModTime()) && cached.size == info.Size() {
		return cloneItems(cached.items), nil
	}
	matches, err := filepath.Glob(filepath.Join(s.baseDir, category, "*.json.gz"))
	if err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(matches))
	for _, path := range matches {
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		items = append(items, Item{
			ID:      strings.TrimSuffix(strings.TrimSuffix(filepath.Base(path), ".gz"), ".json"),
			Path:    path,
			ModTime: info.ModTime(),
			Size:    info.Size(),
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ModTime.After(items[j].ModTime) })
	s.mu.Lock()
	s.listCache[category] = listCacheEntry{
		modTime: info.ModTime(),
		size:    info.Size(),
		items:   cloneItems(items),
	}
	s.mu.Unlock()
	return items, nil
}

func (s *FileStore) Load(category, id string, target any) error {
	path, err := s.resultPath(category, id)
	if err != nil {
		return err
	}
	// #nosec G304 -- resultPath validates category/id and constrains paths to the store root.
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	reader, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer reader.Close()

	return json.NewDecoder(reader).Decode(target)
}

func (s *FileStore) LoadProbeSummary(id string, target any) error {
	return s.loadSummary("probes", id, target)
}

func (s *FileStore) LoadBenchSummary(id string, target any) error {
	return s.loadSummary("benches", id, target)
}

func (s *FileStore) cleanup(category string) error {
	if err := validateStoreCategory(category); err != nil {
		return err
	}
	items, err := s.List(category)
	if err != nil {
		return err
	}
	expiry := time.Now().Add(-s.retention)
	for idx, item := range items {
		tooOld := item.ModTime.Before(expiry)
		tooMany := idx >= s.maxResults
		if tooOld || tooMany {
			if err := os.Remove(item.Path); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("cleanup %s: %w", item.Path, err)
			}
			if err := s.removeSummary(category, item.ID); err != nil {
				return err
			}
		}
	}
	s.invalidateListCache(category)
	return nil
}

func (s *FileStore) invalidateListCache(category string) {
	s.mu.Lock()
	delete(s.listCache, category)
	s.mu.Unlock()
}

func cloneItems(items []Item) []Item {
	if len(items) == 0 {
		return []Item{}
	}
	out := make([]Item, len(items))
	copy(out, items)
	return out
}

func (s *FileStore) resultPath(category, id string) (string, error) {
	if err := validateStoreCategory(category); err != nil {
		return "", err
	}
	if !validStoreID.MatchString(id) {
		return "", fmt.Errorf("invalid result id %q", id)
	}
	root := filepath.Clean(filepath.Join(s.baseDir, category))
	full := filepath.Clean(filepath.Join(root, id+".json.gz"))
	if root != full && !strings.HasPrefix(full, root+string(os.PathSeparator)) {
		return "", errors.New("result path escapes store root")
	}
	return full, nil
}

func validateStoreCategory(category string) error {
	if _, ok := validStoreCategories[category]; !ok {
		return fmt.Errorf("invalid storage category %q", category)
	}
	return nil
}

func (s *FileStore) saveSummary(category, id string, data any) error {
	path, err := s.summaryPath(category, id)
	if err != nil {
		return err
	}
	// #nosec G304 -- summaryPath validates category/id and constrains paths to the store root.
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	if err := json.NewEncoder(file).Encode(data); err != nil {
		return err
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return s.updateSummaryIndex(category, func(entries map[string]json.RawMessage) {
		entries[id] = raw
	})
}

func (s *FileStore) loadSummary(category, id string, target any) error {
	if raw, err := s.loadSummaryIndexEntry(category, id); err == nil {
		return json.Unmarshal(raw, target)
	}
	path, err := s.summaryPath(category, id)
	if err != nil {
		return err
	}
	// #nosec G304 -- summaryPath validates category/id and constrains paths to the store root.
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	return json.NewDecoder(file).Decode(target)
}

func (s *FileStore) summaryPath(category, id string) (string, error) {
	if err := validateStoreCategory(category); err != nil {
		return "", err
	}
	if !validStoreID.MatchString(id) {
		return "", fmt.Errorf("invalid result id %q", id)
	}
	dir, ok := summaryDirs[category]
	if !ok {
		return "", fmt.Errorf("summary storage unsupported for category %q", category)
	}
	root := filepath.Clean(filepath.Join(s.baseDir, dir))
	full := filepath.Clean(filepath.Join(root, id+".json"))
	if root != full && !strings.HasPrefix(full, root+string(os.PathSeparator)) {
		return "", errors.New("summary path escapes store root")
	}
	return full, nil
}

func (s *FileStore) removeSummary(category, id string) error {
	path, err := s.summaryPath(category, id)
	if err != nil {
		if strings.Contains(err.Error(), "unsupported") {
			return nil
		}
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("cleanup %s: %w", path, err)
	}
	return s.updateSummaryIndex(category, func(entries map[string]json.RawMessage) {
		delete(entries, id)
	})
}

func (s *FileStore) summaryIndexPath(category string) (string, error) {
	if err := validateStoreCategory(category); err != nil {
		return "", err
	}
	dir, ok := summaryDirs[category]
	if !ok {
		return "", fmt.Errorf("summary storage unsupported for category %q", category)
	}
	return filepath.Join(s.baseDir, dir, "index.json"), nil
}

func (s *FileStore) loadSummaryIndexEntry(category, id string) (json.RawMessage, error) {
	entries, err := s.summaryIndex(category)
	if err != nil {
		return nil, err
	}
	raw, ok := entries[id]
	if !ok {
		return nil, os.ErrNotExist
	}
	return append(json.RawMessage(nil), raw...), nil
}

func (s *FileStore) summaryIndex(category string) (map[string]json.RawMessage, error) {
	path, err := s.summaryIndexPath(category)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]json.RawMessage{}, nil
		}
		return nil, err
	}

	s.mu.RLock()
	cached, ok := s.indexCache[category]
	s.mu.RUnlock()
	if ok && cached.modTime.Equal(info.ModTime()) && cached.size == info.Size() {
		return cloneSummaryIndexEntries(cached.entries), nil
	}

	// #nosec G304 -- summaryIndexPath constrains paths to the store root.
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	entries := map[string]json.RawMessage{}
	if err := json.NewDecoder(file).Decode(&entries); err != nil {
		return nil, err
	}

	s.mu.Lock()
	s.indexCache[category] = summaryIndexCache{
		modTime: info.ModTime(),
		size:    info.Size(),
		entries: cloneSummaryIndexEntries(entries),
	}
	s.mu.Unlock()
	return entries, nil
}

func (s *FileStore) updateSummaryIndex(category string, update func(map[string]json.RawMessage)) error {
	path, err := s.summaryIndexPath(category)
	if err != nil {
		if strings.Contains(err.Error(), "unsupported") {
			return nil
		}
		return err
	}
	entries, err := s.summaryIndex(category)
	if err != nil {
		return err
	}
	update(entries)

	// #nosec G304 -- summaryIndexPath constrains paths to the store root.
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	if err := json.NewEncoder(file).Encode(entries); err != nil {
		return err
	}

	info, err := file.Stat()
	if err != nil {
		s.mu.Lock()
		delete(s.indexCache, category)
		s.mu.Unlock()
		return nil
	}

	s.mu.Lock()
	s.indexCache[category] = summaryIndexCache{
		modTime: info.ModTime(),
		size:    info.Size(),
		entries: cloneSummaryIndexEntries(entries),
	}
	s.mu.Unlock()
	return nil
}

func cloneSummaryIndexEntries(entries map[string]json.RawMessage) map[string]json.RawMessage {
	if len(entries) == 0 {
		return map[string]json.RawMessage{}
	}
	cloned := make(map[string]json.RawMessage, len(entries))
	for key, value := range entries {
		cloned[key] = append(json.RawMessage(nil), value...)
	}
	return cloned
}
