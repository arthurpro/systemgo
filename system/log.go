package system

import (
	"bytes"
	"io"

	log "github.com/sirupsen/logrus"
)

// Maximum number of bytes kept if log buffer
const BUFFER_SIZE = 10000

type debugHook struct{}

func (h *debugHook) Levels() []log.Level {
	return log.AllLevels
}

func (h *debugHook) Fire(e *log.Entry) error {
	log.WithFields(e.Data).Debug("\t", e.Message)
	return nil
}

// Log uses log.Logger to write data to embedded bytes.Buffer
// Keeps up to 10000 bytes of data in-memory
type Log struct {
	*log.Logger
	*bytes.Reader
	buffer *bytes.Buffer
}

// NewLog returns a new log
func NewLog() (l *Log) {
	defer func() {
		l.Logger = &log.Logger{
			Out: l,
			Formatter: &log.TextFormatter{
				FullTimestamp: true,
			},
			Level: log.InfoLevel,
			Hooks: log.LevelHooks{},
		}
		l.Hooks.Add(&debugHook{})
	}()
	return &Log{
		buffer: bytes.NewBuffer(make([]byte, 0, BUFFER_SIZE)),
	}
}

func (l *Log) Len() (n int) {
	return l.buffer.Len()
}

func (l *Log) Cap() (n int) {
	return l.buffer.Cap()
}

func (l *Log) Read(b []byte) (n int, err error) {
	if l.Reader == nil {
		l.Reader = bytes.NewReader(l.buffer.Bytes())
	}
	defer func() {
		if err == nil && l.Reader.Len() == 0 {
			err = io.EOF
			l.Reader = nil
		}
	}()
	return l.Reader.Read(b)
}

func (l *Log) Write(b []byte) (n int, err error) {
	if l.Len()+len(b) <= l.Cap() {
		return l.buffer.Write(b)
	}

	// Make sure that no 'partial' strings are left in buffer, as the buffer capacity is exceeded
	defer func() {
		if err == nil {
			_, err = l.buffer.ReadString('\n')
		}
	}()

	if len(b) >= l.Cap() {
		l.buffer.Reset()
		return l.buffer.Write(b[len(b)-l.Cap():])
	}

	if _, err = l.buffer.Read(make([]byte, len(b)-l.Cap()+l.Len())); err != nil {
		return 0, err
	}

	return l.buffer.Write(b)
}
