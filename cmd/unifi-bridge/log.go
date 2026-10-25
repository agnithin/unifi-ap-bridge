package main

import (
	"os"
)

// log.Logger serializes calls to Write. Limit redirected regular-file logs in
// RAM without changing pipe/terminal output or creating persistent archives.
type boundedLogWriter struct {
	file     *os.File
	maxBytes int64
}

func (w *boundedLogWriter) Write(data []byte) (int, error) {
	info, err := w.file.Stat()
	if err != nil {
		return 0, err
	}
	if info.Mode().IsRegular() && info.Size()+int64(len(data)) > w.maxBytes {
		if err := w.file.Truncate(0); err != nil {
			return 0, err
		}
		if _, err := w.file.Seek(0, 0); err != nil {
			return 0, err
		}
	}
	// A single oversized event keeps its tail within the bound as well.
	original := len(data)
	if info.Mode().IsRegular() && int64(len(data)) > w.maxBytes {
		data = data[int64(len(data))-w.maxBytes:]
	}
	n, err := w.file.Write(data)
	if err == nil {
		return original, nil
	}
	return n, err
}
