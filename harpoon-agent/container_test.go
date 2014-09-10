package main

import (
	"testing"
)

func TestArtifactDetailsValid(t *testing.T) {
	ExpectArtifactDetails(t, "http://foo/bar.tar", "/srv/harpoon/artifacts/foo/bar", "")
	ExpectArtifactDetails(t, "http://foo/bar.tgz", "/srv/harpoon/artifacts/foo/bar", "z")
	ExpectArtifactDetails(t, "http://foo/bar.tar.gz", "/srv/harpoon/artifacts/foo/bar", "z")
	ExpectArtifactDetails(t, "http://foo/bar.tar.bz2", "/srv/harpoon/artifacts/foo/bar", "j")
}

func ExpectArtifactDetails(
	t *testing.T,
	artifactURL string,
	expectedPath string,
	expectedCompression string) {
	path, compression := getArtifactDetails(artifactURL)
	if path != expectedPath {
		t.Errorf("path %q does not equal expected path %q", path, expectedPath)
	}
	if compression != expectedCompression {
		t.Errorf("path %q does not equal expected compression %q", compression, expectedCompression)
	}
}

func TestInvalidArtifactURLPanics(t *testing.T) {
	defer ExpectPanic(t)
	getArtifactDetails("692734hjlk,mnasdf7o689734")
}

func TestArtifactURLWithUnknownCompressionPanics(t *testing.T) {
	defer ExpectPanic(t)
	getArtifactDetails("http://foo/bar.unknowncompresson")
}

func ExpectPanic(t *testing.T) {
	r := recover()
	if r == nil {
		t.Errorf("Test should have paniced")
	}
}
