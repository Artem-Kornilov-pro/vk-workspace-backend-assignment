// Package cache хранит ETag и тела успешных ответов между запусками.
//
// Каждому адресу соответствует файл <dir>/<sha256(url) в hex>.json с объектом
// {"url": "...", "etag": "...", "link": "...", "body": "..."}. Заголовок Link
// хранится потому, что ответ 304 может прийти без него, а он нужен для
// перехода к следующей странице. Токен и заголовки запроса не сохраняются.
package cache

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// Entry — сохранённый успешный ответ.
type Entry struct {
	URL  string `json:"url"`
	ETag string `json:"etag"`
	Link string `json:"link"` // значение заголовка Link, пусто — заголовка не было
	Body string `json:"body"`
}

// Store — кэш в каталоге. Нулевой указатель — кэш отключён.
type Store struct {
	dir string
}

// Open создаёт каталог кэша при необходимости.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Store{dir: dir}, nil
}

func (s *Store) path(url string) string {
	sum := sha256.Sum256([]byte(url))
	return filepath.Join(s.dir, hex.EncodeToString(sum[:])+".json")
}

// Get возвращает запись для адреса. Повреждённая или чужая запись считается отсутствующей.
func (s *Store) Get(url string) (Entry, bool) {
	if s == nil {
		return Entry{}, false
	}
	data, err := os.ReadFile(s.path(url))
	if err != nil {
		return Entry{}, false
	}
	var e Entry
	if err := json.Unmarshal(data, &e); err != nil || e.URL != url || e.ETag == "" {
		return Entry{}, false
	}
	return e, true
}

// Put сохраняет запись атомарно: через временный файл и переименование.
func (s *Store) Put(e Entry) error {
	if s == nil {
		return nil
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(e); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(s.dir, ".tmp-*")
	if err != nil {
		return err
	}
	_, err = tmp.Write(buf.Bytes())
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), s.path(e.URL))
	}
	if err != nil {
		os.Remove(tmp.Name())
	}
	return err
}

// Delete удаляет запись, если она есть.
func (s *Store) Delete(url string) error {
	if s == nil {
		return nil
	}
	if err := os.Remove(s.path(url)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
