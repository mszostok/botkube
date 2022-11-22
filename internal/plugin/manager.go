package plugin

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/hashicorp/go-plugin"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"

	"github.com/kubeshop/botkube/pkg/api"
	"github.com/kubeshop/botkube/pkg/api/executor"
	"github.com/kubeshop/botkube/pkg/config"
	"github.com/kubeshop/botkube/pkg/multierror"
)

const (
	executorPluginName   = "executor"
	executorBinaryPrefix = "executor_"
	dirPerms             = 0o775
	filePerms            = 0o664
)

// pluginMap is the map of plugins we can dispense.
// This map is used in order to identify a plugin called Dispense.
// This map is globally available and must stay consistent in order for all the plugins to work.
var pluginMap = map[string]plugin.Plugin{
	//"source": &source.Plugin{},
	executorPluginName: &executor.Plugin{},
}

type EnabledPlugin struct {
	Client  executor.Executor
	Cleanup func()
}

type Manager struct {
	log             logrus.FieldLogger
	cfg             config.Plugins
	httpClient      *http.Client
	executors       Data
	executorsConfig map[string]config.Executors
}

type Data struct {
	RepoIndex      map[string]map[string]IndexEntry
	EnabledPlugins map[string]EnabledPlugin
}

func NewManager(logger logrus.FieldLogger, cfg config.Plugins, executors map[string]config.Executors) *Manager {
	return &Manager{
		cfg:             cfg,
		httpClient:      newHTTPClient(),
		executorsConfig: executors,
		executors: Data{
			RepoIndex:      map[string]map[string]IndexEntry{},
			EnabledPlugins: map[string]EnabledPlugin{},
		},
		log: logger.WithField("component", "Plugin Manager"),
	}
}

func (m *Manager) Start(ctx context.Context) error {
	if err := m.loadRepositoriesMetadata(ctx); err != nil {
		return err
	}

	if err := m.loadAllEnabledPlugins(ctx); err != nil {
		return err
	}

	return nil
}

func (m *Manager) loadAllEnabledPlugins(ctx context.Context) error {
	// we start executor only once, so collect only unique names
	allEnabledExecutors := map[string]struct{}{}
	for _, groupItems := range m.executorsConfig {
		for name, executor := range groupItems.Plugins {
			if !executor.Enabled {
				continue
			}

			allEnabledExecutors[name] = struct{}{}
		}
	}

	for name := range allEnabledExecutors {
		repoName, pluginName, found := strings.Cut(name, "/")
		if !found {
			return fmt.Errorf("plugin %q doesn't follow required {repo_name}/{plugin_name} syntax", name)
		}
		binPath := filepath.Join(m.cfg.CacheDir, repoName, fmt.Sprintf("%s%s", executorBinaryPrefix, pluginName))

		// FIXME: Find latest version: - not indexing as it's not hot path.
		info := m.executors.RepoIndex[repoName][pluginName]

		if _, err := os.Stat(binPath); os.IsNotExist(err) {
			err := m.downloadPlugin(ctx, binPath, info)
			if err != nil {
				return fmt.Errorf("while fetching plugin %q binary: %w", name, err)
			}
		}

		m.log.WithFields(logrus.Fields{
			"repo":    repoName,
			"plugin":  pluginName,
			"version": info.Version,
			"binPath": binPath,
		}).Info("Registering executor plugin.")

		client, cleanup, err := m.createGRPCClient(binPath)
		if err != nil {
			if cleanup != nil {
				cleanup()
			}
			return fmt.Errorf("while creating gRPC client: %w", err)
		}

		m.executors.EnabledPlugins[name] = EnabledPlugin{
			Client:  client,
			Cleanup: cleanup,
		}
	}

	return nil
}

func (m *Manager) GetExecutor(name string) (executor.Executor, error) {
	client, found := m.executors.EnabledPlugins[name]
	if !found || client.Client == nil {
		return nil, fmt.Errorf("client for plugin %q not found", name)
	}

	return client.Client, nil
}

func (m *Manager) Shutdown() {
	var wg sync.WaitGroup
	for _, p := range m.executors.EnabledPlugins {
		wg.Add(1)

		go func(close func()) {
			if close != nil {
				close()
			}
			wg.Done()
		}(p.Cleanup)
	}

	wg.Wait()
}

func (m *Manager) loadRepositoriesMetadata(ctx context.Context) error {
	for name, url := range m.cfg.Repositories {
		path := filepath.Join(m.cfg.CacheDir, fmt.Sprintf("%s.yaml", name))
		path = filepath.Clean(path)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			err := m.fetchIndex(ctx, path, url)
			if err != nil {
				return fmt.Errorf("while fetching index for %q repository: %w", name, err)
			}
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("while reading index file: %w", err)
		}
		var index Index
		if err := yaml.Unmarshal(data, &index); err != nil {
			return fmt.Errorf("while unmarshaling index: %w", err)
		}

		for _, entry := range index.Entries {
			switch entry.Type {
			case TypeExecutor:
				_, found := m.executors.RepoIndex[name][entry.Name]
				if found { // FIXME: version semver compare
					continue
				}
				if m.executors.RepoIndex[name] == nil {
					m.executors.RepoIndex[name] = map[string]IndexEntry{}
				}
				m.executors.RepoIndex[name][entry.Name] = entry
			case TypeSource:
				// TODO: ...
			}
		}
	}

	return nil
}

func (m *Manager) fetchIndex(ctx context.Context, path, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return fmt.Errorf("while creating request: %w", err)
	}

	res, err := m.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("while executing request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("incorrect status code: %d", res.StatusCode)
	}

	err = os.MkdirAll(filepath.Dir(path), dirPerms)
	if err != nil {
		return fmt.Errorf("while creating directory where repository index should be stored: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, filePerms)
	if err != nil {
		return fmt.Errorf("while creating file: %w", err)
	}
	defer file.Close()

	_, err = io.Copy(file, res.Body)
	if err != nil {
		return fmt.Errorf("while saving index body: %w", err)
	}
	return nil
}

func (*Manager) createGRPCClient(path string) (executor.Executor, func(), error) {
	cli := plugin.NewClient(&plugin.ClientConfig{
		Plugins:          pluginMap,
		Cmd:              exec.Command(path),
		AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
		HandshakeConfig: plugin.HandshakeConfig{
			ProtocolVersion:  executor.ProtocolVersion,
			MagicCookieKey:   api.HandshakeConfig.MagicCookieKey,
			MagicCookieValue: api.HandshakeConfig.MagicCookieValue,
		},
	})

	rpcClient, err := cli.Client()
	if err != nil {
		return nil, cli.Kill, err
	}

	raw, err := rpcClient.Dispense(executorPluginName)
	if err != nil {
		return nil, cli.Kill, err
	}

	concreteCli, ok := raw.(executor.Executor)
	if !ok {
		return nil, cli.Kill, fmt.Errorf("registered client doesn't implemented executor interface")
	}

	return concreteCli, cli.Kill, nil
}

func (m *Manager) downloadPlugin(ctx context.Context, binPath string, info IndexEntry) error {
	err := os.MkdirAll(filepath.Dir(binPath), dirPerms)
	if err != nil {
		return fmt.Errorf("while creating directory where plugin should be stored: %w", err)
	}

	suffix := fmt.Sprintf("%s_%s", runtime.GOOS, runtime.GOARCH)
	getDownloadURL := func() string {
		for _, url := range info.Links {
			if strings.HasSuffix(url, suffix) {
				return url
			}
		}
		return ""
	}

	url := getDownloadURL()
	if url == "" {
		return fmt.Errorf("cannot find download url with suffix %s", suffix)
	}

	m.log.WithFields(logrus.Fields{
		"url":     url,
		"binPath": binPath,
	}).Info("Downloading plugin.")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return fmt.Errorf("while creating request: %w", err)
	}

	res, err := m.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("while executing request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("incorrect status code: %d", res.StatusCode)
	}

	file, err := os.OpenFile(binPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return fmt.Errorf("while creating plugin file: %w", err)
	}

	_, err = io.Copy(file, res.Body)
	file.Close()
	if err != nil {
		err := multierror.Append(err, os.Remove(binPath))
		return fmt.Errorf("while downloading file: %w", err.ErrorOrNil())
	}

	return nil
}
