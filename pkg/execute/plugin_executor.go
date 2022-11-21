package execute

import (
	"context"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"

	"github.com/kubeshop/botkube/internal/plugin"
	"github.com/kubeshop/botkube/pkg/api/executor"
	"github.com/kubeshop/botkube/pkg/config"
)

// PluginExecutor provides functionality to run registered Botkube plugins.
type PluginExecutor struct {
	log           logrus.FieldLogger
	cfg           config.Config
	pluginManager *plugin.Manager
}

// NewPluginExecutor creates a new instance of PluginExecutor.
func NewPluginExecutor(log logrus.FieldLogger, cfg config.Config, manager *plugin.Manager) *PluginExecutor {
	return &PluginExecutor{
		log:           log,
		cfg:           cfg,
		pluginManager: manager,
	}
}

// CanHandle returns true if it's a known plugin executor.
func (e *PluginExecutor) CanHandle(bindings []string, args []string) (bool, error) {
	if len(args) == 0 {
		return false, nil
	}

	cmdName := args[0]

	plugins, _, err := e.getEnabledPlugins(bindings, cmdName)
	if err != nil {
		return false, err
	}

	return len(plugins) > 0, nil
}

// GetCommandPrefix gets verb command with k8s alias prefix.
func (e *PluginExecutor) GetCommandPrefix(args []string) string {
	if len(args) == 0 {
		return ""
	}

	return args[0]
}

// Execute executes plugin executor based on a given command.
func (e *PluginExecutor) Execute(ctx context.Context, bindings []string, args []string, command string) (string, error) {
	e.log.WithFields(logrus.Fields{
		"bindings": command,
		"command":  command,
	}).Debugf("Handling plugin command...")

	var (
		cmdName = args[0]
	)

	plugins, fullPluginName, err := e.getEnabledPlugins(bindings, cmdName)
	if err != nil {
		return "", fmt.Errorf("while collecting enabled plugins: %w", err)
	}

	configs, err := e.collectConfigs(plugins)
	if err != nil {
		return "", fmt.Errorf("while collecting configs: %w", err)
	}

	cli, err := e.pluginManager.GetExecutor(fullPluginName)
	if err != nil {
		return "", fmt.Errorf("while getting concrete plugin client: %w", err)
	}

	// Execute RPC with all data:
	//  - command
	//  - all configuration but in proper order (so it can be merged properly)
	_ = configs
	resp, err := cli.Execute(ctx, &executor.ExecuteRequest{Command: command})
	if err != nil {
		return "", fmt.Errorf("while executing gRPC call: %w", err)
	}

	return resp.Data, nil
}

func (e *PluginExecutor) collectConfigs(plugins []config.PluginExecutor) ([]string, error) {
	var configs []string

	for _, plugin := range plugins {
		if plugin.Config == nil {
			continue
		}

		raw, err := yaml.Marshal(plugin.Config)
		if err != nil {
			return nil, err
		}

		configs = append(configs, string(raw))
	}

	return configs, nil
}

func (e *PluginExecutor) getEnabledPlugins(bindings []string, cmdName string) ([]config.PluginExecutor, string, error) {
	var (
		out            []config.PluginExecutor
		fullPluginName string
	)

	for _, name := range bindings {
		executors, found := e.cfg.PluginsExecutors[name]
		if !found {
			continue
		}

		for key, executor := range executors {
			if !executor.Enabled {
				continue
			}

			_, pluginName, _ := strings.Cut(key, "/") // FIXME: maybe add a shared method/func to manager/plugin pkg?
			if pluginName != cmdName {
				continue
			}

			fullPluginName = key
			out = append(out, executor)
		}
	}

	return out, fullPluginName, nil
}
