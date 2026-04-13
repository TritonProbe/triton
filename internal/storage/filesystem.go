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
	"time"
)

type FileStore struct {
	baseDir    string
	maxResults int
	retention  time.Duration
}

type Item struct {
	ID      string    `json:"id"`
	Path    string    `json:"path"`
	ModTime time.Time `json:"mod_time"`
	Size    int64     `json:"size"`
}

var (
	validStoreCategories = map[string]struct{}{
		"probes":  {},
		"benches": {},
		"certs":   {},
	}
	validStoreID = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
)

func NewFileStore(baseDir string, maxResults int, retention time.Duration) (*FileStore, error) {
	if err := os.MkdirAll(baseDir, 0o750); err != nil {
		return nil, err
	}
	for _, dir := range []string{"probes", "benches", "certs"} {
		if err := os.MkdirAll(filepath.Join(baseDir, dir), 0o750); err != nil {
			return nil, err
		}
	}
	return &FileStore{baseDir: baseDir, maxResults: maxResults, retention: retention}, nil
}

func (s *FileStore) SaveProbe(id string, data any) error {
	return s.save("probes", id, data)
}

func (s *FileStore) SaveBench(id string, data any) error {
	return s.save("benches", id, data)
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
	defer zip.Close()

	if err := json.NewEncoder(zip).Encode(data); err != nil {
		return err
	}
	return s.cleanup(category)
}

func (s *FileStore) List(category string) ([]Item, error) {
	if err := validateStoreCategory(category); err != nil {
		return nil, err
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
		}
	}
	return nil
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
