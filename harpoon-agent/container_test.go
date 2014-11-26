package main

import (
	"testing"
)

func TestValidArtifactURLs(t *testing.T) {
	validArtifactTestCases := []struct {
		url             string
		wantPath        string
		wantCompression string
	}{{
		"http://foo/bar.tar",
		"/srv/harpoon/artifacts/foo/bar",
		"",
	}, {
		"http://foo/bar.tgz",
		"/srv/harpoon/artifacts/foo/bar",
		"z",
	}, {
		"http://foo/bar.tar.gz",
		"/srv/harpoon/artifacts/foo/bar",
		"z",
	}, {
		"http://foo/bar.tar.bz2",
		"/srv/harpoon/artifacts/foo/bar",
		"j",
	},
	}

	for _, test := range validArtifactTestCases {
		path, compression, err := getArtifactDetails(test.url)
		if path != test.wantPath {
			t.Errorf("artifact url %q: path %q does not equal want path %q", test.url, path, test.wantPath)
		}
		if compression != test.wantCompression {
			t.Errorf("artifact url %q: path %q does not equal want compression %q", test.url, compression, test.wantCompression)
		}
		if err != nil {
			t.Errorf("artifact url %q: did not expect error: %s", test.url, err)
		}
	}
}

func TestInvalidArtifactURLs(t *testing.T) {
	invalidArtifactURLs := []string{"692734hjlk,mnasdf7o689734", "http://foo/bar.unknowncompresson"}

	for _, artifactURL := range invalidArtifactURLs {
		path, compression, err := getArtifactDetails(artifactURL)
		if path != "" {
			t.Errorf("artifact url %q: want no path, but got %q", artifactURL, path)
		}
		if compression != "" {
			t.Errorf("artifact url %q: want no compression, but got %q", artifactURL, compression)
		}
		if err == nil {
			t.Errorf("artifact url %q: want error, but got nothing", artifactURL)
		}
	}
}
