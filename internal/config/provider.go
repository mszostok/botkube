package config

import "context"

type YAMLFiles [][]byte

// Provider provides loaded configuration files in YAML format.
type Provider interface {
	Configs(ctx context.Context) (YAMLFiles, error)
}
