package internal

import (
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/kubeshop/botkube/pkg/api/executor"
)

type (
	ExecIndex struct {
		Interactivity []Interactivity `yaml:"interactivity"`
	}

	Command struct {
		Parser string `yaml:"parser"`
		Prefix string `yaml:"prefix"`
	}

	Interactivity struct {
		Message Message `yaml:"message"`
		Command Command `yaml:"command"`
	}

	Message struct {
		Select  Select            `yaml:"select"`
		Actions map[string]string `yaml:"actions"`
		Preview string            `yaml:"preview"`
	}

	Select struct {
		Name    string `yaml:"name"`
		ItemKey string `yaml:"itemKey"`
	}
)

func (e ExecIndex) For(cmd string) (Interactivity, bool) {
	for _, item := range e.Interactivity {
		if strings.HasPrefix(cmd, item.Command.Prefix) {
			return item, true
		}
	}

	return Interactivity{}, false
}
func NewConfig(in []*executor.Config) (ExecIndex, error) {
	if len(in) == 0 {
		return ExecIndex{}, nil
	}

	// FIXME: merge all...
	var cfg ExecIndex
	err := yaml.Unmarshal(in[0].RawYAML, &cfg)
	return cfg, err
}
