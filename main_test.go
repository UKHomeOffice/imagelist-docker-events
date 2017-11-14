package main

import (
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/stretchr/testify/assert"
)

func TestMapRepoDigestsToTags(t *testing.T) {
	s := types.ImageInspect{
		RepoDigests: []string{
			"vaijab/did@sha256:b3bedb83fb69f207d69adffb2e5690b449b658ea2c139240d20f2ef56bcb4c6f",
			"quay.io/vaijab/did@sha256:42c5ace3ac1e133a49d81086993e5817e70c67cdbf808bf8934aac78e3e416f0",
			"quay.io/vaijab/did@sha256:4a967d63c7d5a2da1fd23d5c6733940b2bd8cb1997575d148d5863fd4f460844",
			"quay.io/vaijab/did@sha256:4e2aa0bc2292b0366dbcde7685c26ce81cfda708bab16d32316704da5d9424ee",
		},
		RepoTags: []string{
			"foo/bar:notpushedyet",
			"vaijab/did:v1",
			"quay.io/vaijab/did:v1",
			"quay.io/vaijab/did:v1.1",
			"quay.io/vaijab/did:latest",
			"vaijab/did:latest",
		},
	}

	cases := []struct {
		name       string
		repoDigest string
		tags       []string
	}{
		{
			"quay.io/vaijab/did",
			"quay.io/vaijab/did@sha256:42c5ace3ac1e133a49d81086993e5817e70c67cdbf808bf8934aac78e3e416f0",
			[]string{"v1", "v1.1", "latest"},
		},
		{
			"vaijab/did:v1",
			"vaijab/did@sha256:b3bedb83fb69f207d69adffb2e5690b449b658ea2c139240d20f2ef56bcb4c6f",
			[]string{"v1", "latest"},
		},
		{
			"foo/bar:notpushedyet",
			"",
			[]string{},
		},
	}

	for _, c := range cases {
		m := mapRepoDigestsToTags(c.name, s)
		tags, ok := m[c.repoDigest]
		if c.repoDigest != "" && !ok {
			t.Errorf("failed to match repo digest given an image name %q", c.name)
		}

		if ok {
			assert.Equal(t, c.tags, tags, "failed to match tags for image name %q", c.name)
		}
	}
}
