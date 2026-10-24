// Package trace пишет журнал HTTP-запросов в формате JSON Lines:
// одна строка на каждую попытку запроса.
package trace

import (
	"bytes"
	"encoding/json"
	"io"
	"time"
)

// Значения поля error.
const (
	ErrTimeout = "timeout"
	ErrNetwork = "network"
)

// Entry — одна попытка запроса.
type Entry struct {
	Time      time.Time
	Method    string
	URL       string
	Status    int // 0 — ответа нет
	Attempt   int // с 1
	FromCache bool
	Error     string // пусто — без ошибки, иначе ErrTimeout или ErrNetwork
}

type line struct {
	TS        string  `json:"ts"`
	Method    string  `json:"method"`
	URL       string  `json:"url"`
	Status    int     `json:"status"`
	Attempt   int     `json:"attempt"`
	FromCache bool    `json:"from_cache"`
	Error     *string `json:"error"`
}

// Writer пишет строки журнала. Нулевой указатель — журнал не ведётся.
type Writer struct {
	w   io.Writer
	err error
}

func New(w io.Writer) *Writer {
	return &Writer{w: w}
}

// Record дописывает строку. После первой ошибки записи журнал больше не пишется,
// ошибку возвращает Err.
func (t *Writer) Record(e Entry) {
	if t == nil || t.err != nil {
		return
	}
	l := line{
		TS:        e.Time.UTC().Format(time.RFC3339),
		Method:    e.Method,
		URL:       e.URL,
		Status:    e.Status,
		Attempt:   e.Attempt,
		FromCache: e.FromCache,
	}
	if e.Error != "" {
		l.Error = &e.Error
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if t.err = enc.Encode(l); t.err != nil {
		return
	}
	_, t.err = t.w.Write(buf.Bytes())
}

func (t *Writer) Err() error {
	if t == nil {
		return nil
	}
	return t.err
}
