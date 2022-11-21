package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/kubeshop/botkube/internal/plugin"
)

const (
	basePath             = "http://host.k3d.internal:3000"
	dir                  = "./dist"
	executorBinaryPrefix = "executor_"
)

func main() {
	fs := http.FileServer(http.Dir(dir))
	http.Handle("/static/", http.StripPrefix("/static/", fs))
	http.HandleFunc("/botkube.yaml", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		out, _ := yaml.Marshal(MustBuildIndex())
		w.Write(out)
	})

	log.Print("Listening on :3000...")
	err := http.ListenAndServe(":3000", nil)
	must(err)
}

func MustBuildIndex() plugin.Index {
	files, err := os.ReadDir(dir)
	must(err)

	entries := map[string]plugin.IndexEntry{}
	for _, entry := range files {
		if entry.IsDir() {
			continue
		}
		if !strings.HasPrefix(entry.Name(), executorBinaryPrefix) {
			continue
		}

		name := strings.TrimPrefix(entry.Name(), executorBinaryPrefix)
		name, _, _ = strings.Cut(name, "_")

		item, found := entries[name]
		if !found {
			item = plugin.IndexEntry{
				Name:        name,
				Type:        plugin.TypeExecutor,
				Description: "Executor",
				Version:     "0.1.0",
			}
		}
		item.Links = append(item.Links, fmt.Sprintf("%s/static/%s", basePath, entry.Name()))
		entries[name] = item
	}

	var out plugin.Index
	for _, item := range entries {
		out.Entries = append(out.Entries, item)
	}
	return out
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
