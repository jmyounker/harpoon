package main

import (
	"testing"
)

type artifactDetailsTest struct {
	url                 string
	expectedPath        string
	expectedCompression string
}

func TestValidArtifactURLs(t *testing.T) {
	tests := []artifactDetailsTest{
		artifactDetailsTest{
			url:                 "http://foo/bar.tar",
			expectedPath:        "/srv/harpoon/artifacts/foo/bar",
			expectedCompression: "",
		},
		artifactDetailsTest{
			url:                 "http://foo/bar.tgz",
			expectedPath:        "/srv/harpoon/artifacts/foo/bar",
			expectedCompression: "z",
		},
		artifactDetailsTest{
			url:                 "http://foo/bar.tar.gz",
			expectedPath:        "/srv/harpoon/artifacts/foo/bar",
			expectedCompression: "z",
		},
		artifactDetailsTest{
			url:                 "http://foo/bar.tar.bz2",
			expectedPath:        "/srv/harpoon/artifacts/foo/bar",
			expectedCompression: "j",
		},
	}

	for _, test := range tests {
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
