package logger

import (
	"os"

	"github.com/rs/zerolog"
)

// New returns a zerolog.Logger configured for the application.
func New(debug bool) zerolog.Logger {
	level := zerolog.InfoLevel
	if debug {
		level = zerolog.DebugLevel
	}
	return zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).
		Level(level).
		With().Timestamp().Logger()
}
