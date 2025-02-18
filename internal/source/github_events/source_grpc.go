package github_events

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/kubeshop/botkube/internal/loggerx"
	"github.com/kubeshop/botkube/internal/source/github_events/gh"
	"github.com/kubeshop/botkube/pkg/api"
	"github.com/kubeshop/botkube/pkg/api/source"
)

//go:embed jsonschema.json
var jsonschema string

const (
	// PluginName is the name of the Kubernetes Botkube plugin.
	PluginName = "github-events"

	description = "Watches for GitHub events."
)

// Source Kubernetes source plugin data structure
type Source struct {
	pluginVersion string
}

// NewSource returns a new instance of Source.
func NewSource(version string) *Source {
	return &Source{
		pluginVersion: version,
	}
}

// Stream streams Kubernetes events
func (s *Source) Stream(ctx context.Context, input source.StreamInput) (source.StreamOutput, error) {
	cfg, err := MergeConfigs(input.Configs)
	if err != nil {
		return source.StreamOutput{}, fmt.Errorf("while merging input configs: %w", err)
	}

	out := source.StreamOutput{
		Event: make(chan source.Event),
	}

	ghCli, err := gh.NewClient(&cfg.GitHub, cfg.Log)
	if err != nil {
		return source.StreamOutput{}, fmt.Errorf("while creating GitHub client: %w", err)
	}

	log := loggerx.New(cfg.Log)
	watcher, err := NewWatcher(cfg.RefreshDuration, cfg.Repositories, ghCli, log)
	if err != nil {
		return source.StreamOutput{}, err
	}

	watcher.AsyncConsumeEvents(ctx, &out)

	return out, nil
}

// Metadata returns metadata of Kubernetes configuration
func (s *Source) Metadata(_ context.Context) (api.MetadataOutput, error) {
	return api.MetadataOutput{
		Version:     s.pluginVersion,
		Description: description,
		JSONSchema: api.JSONSchema{
			Value: jsonschema,
		},
	}, nil
}
