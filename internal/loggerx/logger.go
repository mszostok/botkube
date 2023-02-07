package loggerx

import (
	"os"

	"github.com/sirupsen/logrus"
)

// Config holds logger configuration parameters.
type Config struct {
	Level         string `yaml:"level"`
	DisableColors bool   `yaml:"disableColors"`
}

// New returns a new logger based on a given configuration.
func New(cfg Config) logrus.FieldLogger {
	// Only logger the warning severity or above.
	logLevel, err := logrus.ParseLevel(cfg.Level)
	if err != nil {
		// Set Info level as a default
		logLevel = logrus.InfoLevel
	}

	return &logrus.Logger{
		Out:       os.Stdout,
		Formatter: &logrus.TextFormatter{FullTimestamp: true, DisableColors: cfg.DisableColors},
		Hooks:     make(logrus.LevelHooks),
		Level:     logLevel,
		ExitFunc:  os.Exit,
	}
}
