package output

import (
	"fmt"
	"strings"
	"text/template"

	"github.com/huandu/xstrings"

	"github.com/kubeshop/botkube/cmd/executor/exec/internal"
	"github.com/kubeshop/botkube/pkg/bot/interactive"
)

const NoProcessing = "@no-interactivity"

// TODO: Missing bot name + cluster name..
func BuildMessage(cmd, in string, msgCtx internal.Interactivity) (interactive.Message, error) {
	btnBuilder := interactive.ButtonBuilder{BotName: ""}

	if msgCtx.Command.Parser != "table" {
		return interactive.Message{
			Base: interactive.Base{
				Description: "not supported output parser",
			},
		}, nil
	}

	table, lines := ParseTable(in)

	if len(lines) == 0 {
		return interactive.Message{
			Sections: []interactive.Section{
				{
					Base: interactive.Base{
						Description: "Not found.",
					},
				},
			},
		}, nil
	}
	if len(lines) == 1 {
		return interactive.Message{
			Sections: []interactive.Section{
				{
					Base: interactive.Base{
						Description: lines[0],
					},
				},
			},
		}, nil
	}

	// table items
	var dropdowns []interactive.Select
	parent := interactive.Select{
		Type:    interactive.StaticSelect,
		Name:    msgCtx.Message.Select.Name,
		Command: fmt.Sprintf("exec %s", cmd),
	}

	group := interactive.OptionGroup{
		Name: msgCtx.Message.Select.Name,
	}
	for _, row := range table[1:] {
		name, err := render(msgCtx.Message.Select.ItemKey, table[0], row)
		if err != nil {
			return interactive.Message{}, err
		}
		group.Options = append(group.Options, interactive.OptionItem{
			Name:  name,
			Value: fmt.Sprintf("%s -n %s", row[0], row[1]),
		})
	}

	if len(group.Options) > 0 {
		parent.InitialOption = &group.Options[0]
		parent.OptionGroups = []interactive.OptionGroup{group}
		dropdowns = append(dropdowns, parent)
	}

	// preview
	preview := fmt.Sprintf("%s\n%s", lines[0], lines[1]) // just print the first entry
	if msgCtx.Message.Preview != "" {
		prev, err := render(msgCtx.Message.Preview, table[0], table[1])
		if err != nil {
			return interactive.Message{}, err
		}
		preview = prev
	}

	// actions
	var actions []interactive.OptionItem
	for name, tpl := range msgCtx.Message.Actions {
		out, err := render(tpl, table[0], table[1])
		if err != nil {
			return interactive.Message{}, err
		}
		actions = append(actions, interactive.OptionItem{
			Name:  name,
			Value: out,
		})
	}
	return interactive.Message{
		Sections: []interactive.Section{
			{
				Selects: interactive.Selects{
					ID:    "123",
					Items: dropdowns,
				},
			},
			{
				Base: interactive.Base{
					Body: interactive.Body{
						CodeBlock: preview,
					},
				},
			},
			{
				Buttons: []interactive.Button{
					btnBuilder.ForCommandWithoutDesc("Raw output", fmt.Sprintf("exec %s %s", cmd, NoProcessing)),
				},
				Selects: interactive.Selects{
					ID: "1243",
					Items: []interactive.Select{
						{
							Type:    interactive.StaticSelect,
							Name:    "Actions",
							Command: "exec",
							OptionGroups: []interactive.OptionGroup{
								{
									Name:    "Actions",
									Options: actions,
								},
							},
						},
					},
				},
			},
		},
	}, nil
}

func render(tpl string, cols []string, rows []string) (string, error) {
	data := map[string]string{}
	for idx, col := range cols {
		col = xstrings.ToCamelCase(strings.ToLower(col))
		data[col] = rows[idx]
	}
	fmt.Println(data)

	tmpl, err := template.New("tpl").
		Parse(tpl)
	if err != nil {
		return "", err
	}

	var buff strings.Builder
	err = tmpl.Execute(&buff, data)
	if err != nil {
		return "", err
	}

	return buff.String(), nil
}
