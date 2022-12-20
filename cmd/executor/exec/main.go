package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/gookit/color"
	"github.com/hashicorp/go-plugin"
	"github.com/mattn/go-shellwords"

	"github.com/kubeshop/botkube/cmd/executor/exec/internal"
	"github.com/kubeshop/botkube/cmd/executor/exec/internal/output"
	"github.com/kubeshop/botkube/pkg/api"
	"github.com/kubeshop/botkube/pkg/api/executor"
	"github.com/kubeshop/botkube/pkg/format"
)

// version is set via ldflags by GoReleaser.
var version = "dev"

const (
	pluginName = "exec"
)

// InstallExecutor implements Botkube executor plugin.
type InstallExecutor struct{}

// Metadata returns details about Echo plugin.
func (InstallExecutor) Metadata(context.Context) (api.MetadataOutput, error) {
	return api.MetadataOutput{
		Version:     "v1.0.0",
		Description: "Runs installed binaries",
	}, nil
}

var compiledRegex = regexp.MustCompile(`<(https?:\/\/[a-z.0-9\/\-_=]*)>`)

func removeHyperLinks(in string) string {
	matched := compiledRegex.FindAllStringSubmatch(in, -1)
	if len(matched) >= 1 {
		for _, match := range matched {
			if len(match) == 2 {
				in = strings.ReplaceAll(in, match[0], match[1])
			}
		}
	}
	return format.RemoveHyperlinks(in)
}

// Execute returns a given command as response.
func (InstallExecutor) Execute(_ context.Context, in executor.ExecuteInput) (executor.ExecuteOutput, error) {
	cfg, err := internal.NewConfig(in.Configs)
	if err != nil {
		return executor.ExecuteOutput{}, err
	}

	tool := removeHyperLinks(in.Command)

	tool = format.RemoveHyperlinks(in.Command)
	tool = strings.NewReplacer(`“`, `"`, `”`, `"`, `‘`, `"`, `’`, `"`).Replace(tool)

	tool = strings.ReplaceAll(tool, "exec", "")
	tool = strings.TrimSpace(tool)

	var noProcessing bool
	if strings.Contains(tool, output.NoProcessing) {
		tool = strings.ReplaceAll(tool, output.NoProcessing, "")
		noProcessing = true
	}

	out, err := runCmd(tool)
	if err != nil {
		return executor.ExecuteOutput{
			Data: fmt.Sprintf("%s\n%s", out, err.Error()),
		}, nil
	}
	out = color.ClearCode(out)

	if noProcessing {
		return executor.ExecuteOutput{
			Data: out,
		}, nil
	}
	msg, found := cfg.For(tool)
	if !found {
		return executor.ExecuteOutput{
			Data: out,
		}, nil
	}

	interactiveMsg, err := output.BuildMessage(tool, out, msg)
	if err != nil {
		return executor.ExecuteOutput{}, err
	}
	raw, err := json.Marshal(interactiveMsg)
	if err != nil {
		return executor.ExecuteOutput{}, err
	}
	return executor.ExecuteOutput{
		Data: string(raw),
	}, nil
}

func main() {
	executor.Serve(map[string]plugin.Plugin{
		pluginName: &executor.Plugin{
			Executor: &InstallExecutor{},
		},
	})
}

func runCmd(in string) (string, error) {
	args, err := shellwords.Parse(in)
	if err != nil {
		return "", err
	}

	cmd := exec.Command(fmt.Sprintf("/tmp/bin/%s", args[0]), args[1:]...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
