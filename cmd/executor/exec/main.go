package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/hashicorp/go-plugin"
	"github.com/mattn/go-shellwords"

	"github.com/kubeshop/botkube/cmd/executor/exec/internal"
	"github.com/kubeshop/botkube/pkg/api"
	"github.com/kubeshop/botkube/pkg/api/executor"
	"github.com/kubeshop/botkube/pkg/bot/interactive"
	"github.com/kubeshop/botkube/pkg/format"
)

// version is set via ldflags by GoReleaser.
var version = "dev"

const pluginName = "exec"

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
	tool := removeHyperLinks(in.Command)

	tool = format.RemoveHyperlinks(in.Command)
	tool = strings.NewReplacer(`“`, `"`, `”`, `"`, `‘`, `"`, `’`, `"`).Replace(tool)

	tool = strings.ReplaceAll(tool, "exec", "")
	tool = strings.TrimSpace(tool)

	out, err := runCmd(tool)
	if err != nil {
		return executor.ExecuteOutput{
			Data: fmt.Sprintf("%s\n%s", out, err.Error()),
		}, nil
	}

	if !strings.HasPrefix(tool, "helm list") {
		return executor.ExecuteOutput{
			Data: out,
		}, nil
	}

	raw, err := json.Marshal(BuildMessage(out))
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

type ExecIndex struct {
	Interactivity []Interactivity `yaml:"interactivity"`
}
type Command struct {
	Prefix string `yaml:"prefix"`
}
type Table struct {
	Separator string `yaml:"separator"`
}
type Parser struct {
	Table Table `yaml:"table"`
}
type Dropdowns struct {
	Release string `yaml:"release"`
}
type Output struct {
	Parser    Parser    `yaml:"parser"`
	Dropdowns Dropdowns `yaml:"dropdowns"`
}
type Actions struct {
	Delete string `yaml:"delete"`
	Notes  string `yaml:"notes"`
}
type Interactivity struct {
	Command Command `yaml:"command"`
	Output  Output  `yaml:"output"`
	Actions Actions `yaml:"actions"`
}

// Missing bot name + cluster name..
func BuildMessage(in string) interactive.Message {
	table, lines := internal.ParseTable(in)

	var opts []interactive.OptionItem
	for _, row := range table[1:] {
		opts = append(opts, interactive.OptionItem{
			Name:  fmt.Sprintf("%s/%s", row[1], row[0]),
			Value: fmt.Sprintf("%s -n %s", row[0], row[1]),
		})
	}

	btnBuilder := interactive.ButtonBuilder{BotName: "<name>"}
	return interactive.Message{
		Base: interactive.Base{ // TODO: by Botkube core...
			Description: "`exec helm list` on `labs`",
		},
		Sections: []interactive.Section{
			{
				Selects: interactive.Selects{
					ID: "123",
					Items: []interactive.Select{
						{
							Type:          interactive.StaticSelect,
							Name:          "release",
							Command:       fmt.Sprintf("%s helm list", "<bot>"),
							InitialOption: &opts[0],
							OptionGroups: []interactive.OptionGroup{
								{
									Name:    "release",
									Options: opts,
								},
							},
						},
					},
				},
			},
			{
				Base: interactive.Base{
					Body: interactive.Body{
						CodeBlock: fmt.Sprintf("%s\n%s", lines[0], lines[1]), // just print the first entry
					},
				},
			},
			{
				Buttons: []interactive.Button{
					btnBuilder.ForCommandWithoutDesc("Raw output", "exec helm list --no-interactivity"),
				},
				Selects: interactive.Selects{
					ID: "1243",
					Items: []interactive.Select{
						{
							Type:    interactive.StaticSelect,
							Name:    "release",
							Command: fmt.Sprintf("%s exec helm", "<bot>"),
							OptionGroups: []interactive.OptionGroup{
								{
									Name: "Actions",
									Options: []interactive.OptionItem{
										{
											Name:  "delete",
											Value: fmt.Sprintf("delete %s -n %s", table[1][0], table[1][1]),
										},
										{
											Name:  "notes",
											Value: fmt.Sprintf("get notes %s -n %s", table[1][0], table[1][1]),
										},
									},
								},
							},
							//InitialOption: nil,
						},
					},
				},
			},
		},
	}
}
