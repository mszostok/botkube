package plugin

import "testing"

func TestManager(t *testing.T) {

	m := Manager{}

	idx := Index{
		Entries: []IndexEntry{
			{
				Name:        "kubernetes",
				Type:        "source",
				Description: "Kubernetes source plugin to consume events from Kubernetes Events API.",
				Version:     "v1.0.9",
			},
			{
				Name:        "kubectl",
				Type:        "executor",
				Description: "Kubectl executor plugin to handle given command in configured k8s cluster.",
				Version:     "v1.0.9",
			},
		},
	}

	m.EnableSources([]Plugin{
		{
			Name:    "kubectl",
			Version: "v1.0.9",
		},
	})

	m.EnableExecutors([]Plugin{
		{
			Name: "kubernetes",
		},
	})
}
