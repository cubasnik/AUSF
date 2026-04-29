package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

type AuthContextStore interface {
	NextAuthCtxID() (string, error)
	Save(context AuthContext) error
	Get(authCtxID string) (AuthContext, bool, error)
	Delete(authCtxID string) (bool, error)
}

type InMemoryAuthContextStore struct {
	mu       sync.RWMutex
	contexts map[string]AuthContext
	serial   uint64
}

func NewInMemoryAuthContextStore() *InMemoryAuthContextStore {
	return &InMemoryAuthContextStore{contexts: make(map[string]AuthContext)}
}

func (store *InMemoryAuthContextStore) NextAuthCtxID() (string, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	store.serial++
	return formatAuthCtxID(store.serial), nil
}

func (store *InMemoryAuthContextStore) Save(context AuthContext) error {
	store.mu.Lock()
	defer store.mu.Unlock()

	store.contexts[context.AuthCtxID] = context
	return nil
}

func (store *InMemoryAuthContextStore) Get(authCtxID string) (AuthContext, bool, error) {
	store.mu.RLock()
	defer store.mu.RUnlock()

	context, ok := store.contexts[authCtxID]
	return context, ok, nil
}

func (store *InMemoryAuthContextStore) Delete(authCtxID string) (bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	if _, ok := store.contexts[authCtxID]; !ok {
		return false, nil
	}
	delete(store.contexts, authCtxID)
	return true, nil
}

type fileStoreState struct {
	Serial   uint64                 `json:"serial"`
	Contexts map[string]AuthContext `json:"contexts"`
}

type FileAuthContextStore struct {
	mu    sync.RWMutex
	path  string
	state fileStoreState
}

func NewFileAuthContextStore(path string) (*FileAuthContextStore, error) {
	store := &FileAuthContextStore{
		path: path,
		state: fileStoreState{
			Contexts: make(map[string]AuthContext),
		},
	}

	if err := store.load(); err != nil {
		return nil, err
	}

	return store, nil
}

func (store *FileAuthContextStore) NextAuthCtxID() (string, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	store.state.Serial++
	if err := store.flushLocked(); err != nil {
		store.state.Serial--
		return "", err
	}
	return formatAuthCtxID(store.state.Serial), nil
}

func (store *FileAuthContextStore) Save(context AuthContext) error {
	store.mu.Lock()
	defer store.mu.Unlock()

	store.state.Contexts[context.AuthCtxID] = context
	if err := store.flushLocked(); err != nil {
		delete(store.state.Contexts, context.AuthCtxID)
		return err
	}
	return nil
}

func (store *FileAuthContextStore) Get(authCtxID string) (AuthContext, bool, error) {
	store.mu.RLock()
	defer store.mu.RUnlock()

	context, ok := store.state.Contexts[authCtxID]
	return context, ok, nil
}

func (store *FileAuthContextStore) Delete(authCtxID string) (bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	if _, ok := store.state.Contexts[authCtxID]; !ok {
		return false, nil
	}
	context := store.state.Contexts[authCtxID]
	delete(store.state.Contexts, authCtxID)
	if err := store.flushLocked(); err != nil {
		store.state.Contexts[authCtxID] = context
		return false, err
	}
	return true, nil
}

func (store *FileAuthContextStore) load() error {
	raw, err := os.ReadFile(store.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read auth context store: %w", err)
	}

	var decoded fileStoreState
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return fmt.Errorf("decode auth context store: %w", err)
	}

	if decoded.Contexts == nil {
		decoded.Contexts = make(map[string]AuthContext)
	}
	if decoded.Serial < inferSerialFromContexts(decoded.Contexts) {
		decoded.Serial = inferSerialFromContexts(decoded.Contexts)
	}

	store.state = decoded
	return nil
}

func (store *FileAuthContextStore) flushLocked() error {
	if err := os.MkdirAll(filepath.Dir(store.path), 0o755); err != nil {
		return fmt.Errorf("prepare auth context store directory: %w", err)
	}

	raw, err := json.MarshalIndent(store.state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode auth context store: %w", err)
	}

	tempPath := store.path + ".tmp"
	if err := os.WriteFile(tempPath, raw, 0o644); err != nil {
		return fmt.Errorf("write auth context store temp file: %w", err)
	}

	if err := os.Rename(tempPath, store.path); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("replace auth context store: %w", err)
	}

	return nil
}

func inferSerialFromContexts(contexts map[string]AuthContext) uint64 {
	var serial uint64
	for authCtxID := range contexts {
		candidate := parseAuthCtxSerial(authCtxID)
		if candidate > serial {
			serial = candidate
		}
	}
	return serial
}

func parseAuthCtxSerial(authCtxID string) uint64 {
	if !strings.HasPrefix(authCtxID, "auth-") {
		return 0
	}
	value := strings.TrimPrefix(authCtxID, "auth-")
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0
	}
	return parsed
}

func formatAuthCtxID(serial uint64) string {
	return fmt.Sprintf("auth-%d", serial)
}
