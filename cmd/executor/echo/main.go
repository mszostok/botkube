package main

import (
	"context"

	"github.com/hashicorp/go-plugin"

	"github.com/kubeshop/botkube/pkg/api/executor"
)

const pluginName = "echo"

// EchoExecutor implements Botkube executor plugin
type EchoExecutor struct{}

// Execute returns a given command as response.
func (EchoExecutor) Execute(_ context.Context, req *executor.ExecuteRequest) (*executor.ExecuteResponse, error) {
	return &executor.ExecuteResponse{Data: req.Command}, nil
}

func main() {
	executor.Serve(map[string]plugin.Plugin{
		pluginName: &executor.Plugin{
			Executor: &EchoExecutor{},
		},
	})
}
