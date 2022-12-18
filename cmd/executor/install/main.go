package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/hashicorp/go-plugin"
	"github.com/mattn/go-shellwords"

	"github.com/kubeshop/botkube/pkg/api"
	"github.com/kubeshop/botkube/pkg/api/executor"
	"github.com/kubeshop/botkube/pkg/format"
)

// version is set via ldflags by GoReleaser.
var version = "dev"

const pluginName = "install"

// InstallExecutor implements Botkube executor plugin.
type InstallExecutor struct{}

// Metadata returns details about Echo plugin.
func (InstallExecutor) Metadata(context.Context) (api.MetadataOutput, error) {
	return api.MetadataOutput{
		Version:     "v1.0.0",
		Description: "Downloads and installs pre-built binaries from releases on GitHub",
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
	tool := removeHyperLinks(in.Command)
	tool = strings.NewReplacer(`“`, `"`, `”`, `"`, `‘`, `"`, `’`, `"`).Replace(tool)

	tool = strings.ReplaceAll(tool, "install", "")
	fmt.Println("in ", in.Command)
	fmt.Println("tool ", tool)
	tool = strings.TrimSpace(tool)
	install(tool)

	return executor.ExecuteOutput{
		Data: fmt.Sprintf("Scheduled %s installation...", tool),
	}, nil
}

func main() {
	executor.Serve(map[string]plugin.Plugin{
		pluginName: &executor.Plugin{
			Executor: &InstallExecutor{},
		},
	})
}

func install(tool string) {
	fmt.Println("in tool", tool)
	cmd := fmt.Sprintf("/tmp/bin/eget %s --to=/tmp/bin", tool)
	fmt.Printf("running %s\n", cmd)
	go func() {
		err := os.MkdirAll("/tmp/bin", 0o777)
		if err != nil {
			log.Println("while creating it", err)
			return
		}

		out, err := runCmd("wget -O /tmp/bin/eget https://github.com/mszostok/botkube/releases/download/v0.66.0/eget")
		if err != nil {
			log.Println("while downloading it", out, err)
			return
		}

		out, err = runCmd("chmod +x /tmp/bin/eget")
		if err != nil {
			log.Println("while changing mod", out, err)
			return
		}

		out, err = runCmdTool(cmd)
		if err != nil {
			log.Println("while running mod", out, err)
			return
		}

		fmt.Println(out)
	}()
}

func runCmdTool(in string) (string, error) {
	args, err := shellwords.Parse(in)
	if err != nil {
		return "", err
	}

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "PATH=$PATH:/tmp/bin")

	out, err := cmd.CombinedOutput()
	return string(out), err
}

func runCmd(in string) (string, error) {
	args, err := shellwords.Parse(in)
	if err != nil {
		return "", err
	}

	cmd := exec.Command(args[0], args[1:]...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
