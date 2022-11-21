package plugin

type Type string

const (
	TypeSource   Type = "source"
	TypeExecutor Type = "executor"
)

type (
	Index struct {
		Entries []IndexEntry
	}
	IndexEntry struct {
		Name        string
		Type        Type
		Description string
		Version     string
		Links       []string
	}
)

type Plugin struct {
	Name    string
	Version string
}
