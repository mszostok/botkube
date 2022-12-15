package main

import (
	"context"
	"fmt"
	"log"
	"os/exec"

	"github.com/hashicorp/go-plugin"
	"github.com/mattn/go-shellwords"

	"github.com/kubeshop/botkube/pkg/api"
	"github.com/kubeshop/botkube/pkg/api/executor"
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

// Execute returns a given command as response.
func (InstallExecutor) Execute(_ context.Context, in executor.ExecuteInput) (executor.ExecuteOutput, error) {
	cmd := fmt.Sprintf("eget %s --to=/usr/local/bin", in.Command)
	go func() {
		_, err := runCmd("wget -O /usr/local/bin/eget https://github.com/mszostok/botkube/releases/download/v0.66.0/eget")
		exitOnErr(err)

		_, err = runCmd("chmod +x /usr/local/bin/eget")
		exitOnErr(err)

		out, err := runCmd(cmd)
		if err != nil {
			fmt.Println(err)
		}
		fmt.Println(out)
	}()

	return executor.ExecuteOutput{
		Data: cmd,
	}, nil
}

func main() {
	executor.Serve(map[string]plugin.Plugin{
		pluginName: &executor.Plugin{
			Executor: &InstallExecutor{},
		},
	})
}

func runCmd(in string) ([]byte, error) {
	args, err := shellwords.Parse(in)
	if err != nil {
		return nil, err
	}

	out, err := exec.Command(args[0], args[1:]...).CombinedOutput()
	if err != nil {
		return nil, err
	}

	return out, nil
}

func exitOnErr(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
