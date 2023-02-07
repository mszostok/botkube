package kubectl

import (
	"context"
	"fmt"
	"github.com/kubeshop/botkube/internal/executor/kubectl/builder"
	"github.com/kubeshop/botkube/pkg/config"
	"github.com/kubeshop/botkube/pkg/execute/kubectl"
	"github.com/sirupsen/logrus"
	"github.com/slack-go/slack"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"os"
	"strings"

	"github.com/kubeshop/botkube/pkg/api"
	"github.com/kubeshop/botkube/pkg/api/executor"
)

const (
	// PluginName is the name of the Helm Botkube plugin.
	PluginName       = "kubectl"
	defaultNamespace = "default"
	description      = "Kubectl is the Botkube executor plugin that allows you to run the Kubectl CLI commands directly from any communication platform."
)

var kcBinaryDownloadLinks = map[string]string{
	"windows/amd64": "https://dl.k8s.io/release/v1.26.0/bin/windows/amd64/kubectl.exe",
	"darwin/amd64":  "https://dl.k8s.io/release/v1.26.0/bin/darwin/amd64/kubectl",
	"darwin/arm64":  "https://dl.k8s.io/release/v1.26.0/bin/darwin/arm64/kubectl",
	"linux/amd64":   "https://dl.k8s.io/release/v1.26.0/bin/linux/amd64/kubectl",
	"linux/s390x":   "https://dl.k8s.io/release/v1.26.0/bin/linux/s390x/kubectl",
	"linux/ppc64le": "https://dl.k8s.io/release/v1.26.0/bin/linux/ppc64le/kubectl",
	"linux/arm64":   "https://dl.k8s.io/release/v1.26.0/bin/linux/arm64/kubectl",
	"linux/386":     "https://dl.k8s.io/release/v1.26.0/bin/linux/386/kubectl",
}

var _ executor.Executor = &Executor{}

type (
	kcRunner interface {
		RunKubectlCommand(ctx context.Context, defaultNamespace, cmd string) (string, error)
	}
	kcBuilder interface {
		Do(ctx context.Context, cmd, defaultNamespace string, platform config.CommPlatformIntegration, state *slack.BlockActionStates, botName string, header string) (api.Message, error)
	}
)

// Executor provides functionality for running Helm CLI.
type Executor struct {
	pluginVersion string
	kcRunner      kcRunner

	logger    logrus.FieldLogger
	kcBuilder *builder.KubectlCmdBuilder
}

// NewExecutor returns a new Executor instance.
func NewExecutor(logger logrus.FieldLogger, ver string, kcRunner kcRunner) *Executor {
	return &Executor{
		pluginVersion: ver,
		logger:        logger,
		kcBuilder:     builder.NewKubectlCmdBuilder(logger, kcRunner), // TODO: logger
		kcRunner:      kcRunner,
	}
}

// Metadata returns details about Helm plugin.
func (e *Executor) Metadata(context.Context) (api.MetadataOutput, error) {
	return api.MetadataOutput{
		Version:     e.pluginVersion,
		Description: description,
		//JSONSchema:  jsonSchema(),
		Dependencies: map[string]api.Dependency{
			binaryName: {
				URLs: kcBinaryDownloadLinks,
			},
		},
	}, nil
}

// Execute returns a given command as response.
func (e *Executor) Execute(ctx context.Context, in executor.ExecuteInput) (executor.ExecuteOutput, error) {
	cfg, err := MergeConfigs(in.Configs)
	if err != nil {
		return executor.ExecuteOutput{}, fmt.Errorf("while merging input configs: %w", err)
	}

	cmd, err := normalizeCommand(in.Command)
	if err != nil {
		return executor.ExecuteOutput{}, err
	}

	l := logrus.New()
	l.SetLevel(logrus.DebugLevel)
	l.SetOutput(os.Stdout)

	l.Infoln("kura")
	e.logger = l
	e.logger.Infoln("kurłaa")

	fmt.Println(cmd)
	if cmd == "" || strings.HasPrefix(cmd, "@builder") {
		kubeConfig, err := clientcmd.BuildConfigFromFlags("", os.Getenv("KUBECONFIG")) // TODO: from execution context
		if err != nil {
			return executor.ExecuteOutput{}, fmt.Errorf("while merging input configs: %w", err)
		}

		discoveryClient, err := discovery.NewDiscoveryClientForConfig(kubeConfig)
		if err != nil {
			return executor.ExecuteOutput{}, fmt.Errorf("while merging input configs: %w", err)
		}
		discoCacheClient := memory.NewMemCacheClient(discoveryClient)
		guard := kubectl.NewCommandGuard(e.logger, discoCacheClient)
		k8sCli, err := kubernetes.NewForConfig(kubeConfig)
		if err != nil {
			return executor.ExecuteOutput{}, fmt.Errorf("while merging input configs: %w", err)
		}

		e.kcBuilder.CommandGuard = guard
		e.kcBuilder.NamespaceLister = k8sCli.CoreV1().Namespaces()
		// let's start interactive mode, or continue existing one
		msg, err := e.kcBuilder.Do(ctx, l, cmd, cfg.DefaultNamespace, in.Context.CommunicationPlatform, in.Context.SlackState, "@Botkube")
		if err != nil {
			return executor.ExecuteOutput{}, fmt.Errorf("while running command builder: %w", err)
		}
		return executor.ExecuteOutput{
			Message: msg,
		}, nil
	}

	out, err := e.kcRunner.RunKubectlCommand(ctx, cfg.DefaultNamespace, cmd)
	if err != nil {
		return executor.ExecuteOutput{}, err
	}
	return executor.ExecuteOutput{
		Message: api.NewCodeBlockMessage(out, true),
	}, nil
}

// isNamespaceFlagSet returns true if `--namespace/-n` was found.
func isNamespaceFlagSet(cmd string) bool {
	return strings.Contains(cmd, "-n") || strings.Contains(cmd, "--namespace")
}

// Help returns help message
func (*Executor) Help(_ context.Context) (api.Message, error) {
	return api.Message{
		Base: api.Base{
			Body: api.Body{
				CodeBlock: help(),
			},
		},
	}, nil
}
