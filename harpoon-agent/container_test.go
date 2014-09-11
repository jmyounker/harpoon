package main

import (
	"testing"
)

func TestValidArtifactURLs(t *testing.T) {
	validArtifactTestCases := []struct {
		url                 string
		expectedPath        string
		expectedCompression string
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
		if path != test.expectedPath {
			t.Errorf("artifact url %q: path %q does not equal expected path %q", test.url, path, test.expectedPath)
		}
		if compression != test.expectedCompression {
			t.Errorf("artifact url %q: path %q does not equal expected compression %q", test.url, compression, test.expectedCompression)
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
			t.Errorf("artifact url %q: expected no path, but got %q", artifactURL, path)
		}
		if compression != "" {
			t.Errorf("artifact url %q: expected no compression, but got %q", artifactURL, compression)
		}
		if err == nil {
			t.Errorf("artifact url %q: expected error, but got nothing", artifactURL)
		}
	}
}
