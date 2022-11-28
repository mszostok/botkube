package plugin

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStoreRepository(t *testing.T) {
	// given
	repositories := map[string][]byte{
		"botkube":  loadTestdataFile(t, "botkube.yaml"),
		"mszostok": loadTestdataFile(t, "mszostok.yaml"),
	}

	expectedExecutors := storeRepository{
		"botkube/kubectl": {
			{
				Description: "Kubectl executor plugin.",
				Version:     "v1.5.0",
				URLs: map[string]string{
					"darwin/amd64": "https://github.com/kubeshop/botkube/releases/download/v0.27.0/executor_kubectl-darwin-amd64",
					"darwin/arm64": "https://github.com/kubeshop/botkube/releases/download/v0.27.0/executor_kubectl-darwin-arm64",
					"linux/amd64":  "https://github.com/kubeshop/botkube/releases/download/v0.27.0/executor_kubectl-linux-amd64",
					"linux/arm64":  "https://github.com/kubeshop/botkube/releases/download/v0.27.0/executor_kubectl-linux-arm64",
				},
			},
			{
				Description: "Kubectl executor plugin.",
				Version:     "v1.0.0",
				URLs: map[string]string{
					"darwin/amd64": "https://github.com/kubeshop/botkube/releases/download/v0.17.0/executor_kubectl-darwin-amd64",
					"darwin/arm64": "https://github.com/kubeshop/botkube/releases/download/v0.17.0/executor_kubectl-darwin-arm64",
					"linux/amd64":  "https://github.com/kubeshop/botkube/releases/download/v0.17.0/executor_kubectl-linux-amd64",
					"linux/arm64":  "https://github.com/kubeshop/botkube/releases/download/v0.17.0/executor_kubectl-linux-arm64",
				},
			},
		},
		"mszostok/echo": {
			{
				Description: "Executor suitable for e2e testing. It returns the command that was send as an input.",
				Version:     "v1.0.2",
				URLs: map[string]string{
					"darwin/amd64": "https://github.com/mszostok/botkube-plugins/releases/download/v1.0.2/executor_echo-darwin-amd64",
					"darwin/arm64": "https://github.com/mszostok/botkube-plugins/releases/download/v1.0.2/executor_echo-darwin-arm64",
					"linux/amd64":  "https://github.com/mszostok/botkube-plugins/releases/download/v1.0.2/executor_echo-linux-amd64",
					"linux/arm64":  "https://github.com/mszostok/botkube-plugins/releases/download/v1.0.2/executor_echo-linux-arm64",
				},
			},
		},
	}

	// when
	executors, _, err := newStoreRepositories(repositories)

	// then
	require.NoError(t, err)
	assert.Equal(t, executors, expectedExecutors)
}

func loadTestdataFile(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join("testdata", t.Name(), name)

	raw, err := os.ReadFile(filepath.Clean(path))
	require.NoError(t, err)

	return raw
}
