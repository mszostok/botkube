package flux

import (
	"context"
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/allegro/bigcache/v3"

	"github.com/kubeshop/botkube/internal/executor/flux/commands"
	"github.com/kubeshop/botkube/internal/executor/x"
	"github.com/kubeshop/botkube/internal/executor/x/output"
	"github.com/kubeshop/botkube/internal/executor/x/state"
	"github.com/kubeshop/botkube/internal/loggerx"
	"github.com/kubeshop/botkube/pkg/api"
	"github.com/kubeshop/botkube/pkg/api/executor"
	"github.com/kubeshop/botkube/pkg/pluginx"
)

const (
	PluginName  = "flux"
	description = "Run the Flux CLI commands directly from your favorite communication platform."
)

// Executor provides functionality for running Flux.
type Executor struct {
	pluginVersion string
	cache         *bigcache.BigCache
}

// NewExecutor returns a new Executor instance.
func NewExecutor(cache *bigcache.BigCache, ver string) *Executor {
	x.BuiltinCmdPrefix = "" // we don't need them
	return &Executor{
		pluginVersion: ver,
		cache:         cache,
	}
}

// Metadata returns details about the Flux plugin.
func (d *Executor) Metadata(context.Context) (api.MetadataOutput, error) {
	return api.MetadataOutput{
		Version:      d.pluginVersion,
		Description:  description,
		Dependencies: getPluginDependencies(),
		JSONSchema:   jsonSchema(),
	}, nil
}

func getPluginDependencies() map[string]api.Dependency {
	return map[string]api.Dependency{
		"flux": {
			URLs: map[string]string{
				"windows/amd64": "https://github.com/fluxcd/flux2/releases/download/v2.0.1/flux_2.0.1_windows_amd64.zip",
				"windows/arm64": "https://github.com/fluxcd/flux2/releases/download/v2.0.1/flux_2.0.1_windows_arm64.zip",
				"darwin/amd64":  "https://github.com/fluxcd/flux2/releases/download/v2.0.1/flux_2.0.1_darwin_amd64.tar.gz",
				"darwin/arm64":  "https://github.com/fluxcd/flux2/releases/download/v2.0.1/flux_2.0.1_darwin_arm64.tar.gz",
				"linux/amd64":   "https://github.com/fluxcd/flux2/releases/download/v2.0.1/flux_2.0.1_linux_amd64.tar.gz",
				"linux/arm64":   "https://github.com/fluxcd/flux2/releases/download/v2.0.1/flux_2.0.1_linux_arm64.tar.gz",
			},
		},
		"gh": {
			URLs: map[string]string{
				"windows/amd64": "https://github.com/cli/cli/releases/download/v2.32.1/gh_2.32.1_windows_amd64.zip//gh_2.32.1_windows_amd64/bin",
				"windows/arm64": "https://github.com/cli/cli/releases/download/v2.32.1/gh_2.32.1_windows_arm64.zip//gh_2.32.1_windows_arm64/bin",
				"darwin/amd64":  "https://github.com/cli/cli/releases/download/v2.32.1/gh_2.32.1_macOS_amd64.zip//gh_2.32.1_macOS_amd64/bin",
				"darwin/arm64":  "https://github.com/cli/cli/releases/download/v2.32.1/gh_2.32.1_macOS_arm64.zip//gh_2.32.1_macOS_arm64/bin",
				"linux/amd64":   "https://github.com/cli/cli/releases/download/v2.32.1/gh_2.32.1_linux_amd64.tar.gz//gh_2.32.1_linux_amd64/bin",
				"linux/arm64":   "https://github.com/cli/cli/releases/download/v2.32.1/gh_2.32.1_linux_arm64.tar.gz//gh_2.32.1_linux_arm64/bin",
			},
		},
	}
}

// Execute returns a given command as a response.
func (d *Executor) Execute(ctx context.Context, in executor.ExecuteInput) (executor.ExecuteOutput, error) {
	cmd := normalize(in.Command)

	if err := detectNotSupportedGlobalFlags(cmd); err != nil {
		return executor.ExecuteOutput{}, err
	}

	if err := pluginx.ValidateKubeConfigProvided(PluginName, in.Context.KubeConfig); err != nil {
		return executor.ExecuteOutput{}, err
	}

	var cfg Config
	err := pluginx.MergeExecutorConfigs(in.Configs, &cfg)
	if err != nil {
		return executor.ExecuteOutput{}, fmt.Errorf("while merging input configuration: %w", err)
	}

	log := loggerx.New(cfg.Logger)

	kubeConfigPath, deleteFn, err := pluginx.PersistKubeConfig(ctx, in.Context.KubeConfig)
	if err != nil {
		return executor.ExecuteOutput{}, fmt.Errorf("while writing kubeconfig file: %w", err)
	}
	defer func() {
		if deleteErr := deleteFn(ctx); deleteErr != nil {
			log.Errorf("failed to delete kubeconfig file %s: %w", kubeConfigPath, deleteErr)
		}
	}()

	log.WithField("rawCommand", cmd).Info("Processing command...")

	diffHandler := NewKustomizeDiffCmdService(d.cache, log)
	if diffCmd, shouldHandle := diffHandler.ShouldHandle(in.Command); shouldHandle {
		return diffHandler.Run(ctx, diffCmd, kubeConfigPath, in.Context.KubeConfig, cfg)
	}

	ghHandler := NewGitHubCmdService(log)
	if ghCmd, shouldHandle := ghHandler.ShouldHandle(in.Command); shouldHandle {
		return ghHandler.Run(ctx, ghCmd, cfg, nil)
	}

	renderer := x.NewRenderer()
	err = renderer.RegisterAll(map[string]x.Render{
		"parser:table:.*": output.NewTableCommandParser(log),
		"wrapper":         output.NewCommandWrapper(),
		"tutorial":        output.NewTutorialWrapper(),
	})
	if err != nil {
		return executor.ExecuteOutput{}, fmt.Errorf("while registering message renderers: %v", err)
	}

	command := x.Parse(cmd)

	templates, err := commands.LoadTemplates()
	if err != nil {
		return executor.ExecuteOutput{}, fmt.Errorf("while loading templates: %w", err)
	}

	return x.NewRunner(log, renderer).RunWithTemplates(templates, state.ExtractSlackState(in.Context.SlackState), command, func() (string, error) {
		out, err := ExecuteCommand(ctx, command.ToExecute, pluginx.ExecuteCommandEnvs(map[string]string{
			"KUBECONFIG": kubeConfigPath,
		}))
		if err != nil {
			log.WithError(err).WithField("command", command.ToExecute).Error("failed to run command")
			return "", fmt.Errorf("while running command: %v", err)
		}
		return out, nil
	})
}

// Help returns help message
func (d *Executor) Help(context.Context) (api.Message, error) {
	renderer := x.NewRenderer()
	err := renderer.Register("tutorial", output.NewTutorialWrapper())
	if err != nil {
		return api.Message{}, fmt.Errorf("while registering message renderers: %v", err)
	}

	runner := x.NewRunner(loggerx.NewNoop(), renderer)

	templates, err := commands.LoadTemplates()
	if err != nil {
		return api.Message{}, err
	}

	out, err := runner.RunWithTemplates(templates, nil, x.Parse("flux tutorial"), func() (string, error) {
		return "", nil
	})

	if err != nil {
		return api.Message{}, err
	}

	return out.Message, nil
}

// jsonSchema returns JSON schema for the executor.
func jsonSchema() api.JSONSchema {
	return api.JSONSchema{
		Value: heredoc.Docf(`{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "title": "Flux",
  "description": "%s",
  "type": "object",
  "properties": {
    "github": {
      "title": "GitHub",
      "type": "object",
      "properties": {
        "auth": {
          "title": "Auth",
          "type": "object",
          "properties": {
            "accessToken": {
              "title": "Access Token",
              "description": "The GitHub access token. When not provided, some functionality may not work. For example, adding a comment under a pull request.",
              "type": "string"
            }
          }
        }
      }
    }
  },
  "required": []
}
`, description),
	}
}
