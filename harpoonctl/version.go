package main

import "fmt"

var (
	// Version is set by the Makefile.
	Version = ""

	// CommitID is set by the Makefile.
	CommitID = ""

	// ExternalReleaseVersion is set by the Makefile.
	ExternalReleaseVersion = ""
)

func version() string {
	if Version == "" && CommitID == "" && ExternalReleaseVersion == "" {
		return "development"
	}
	return fmt.Sprintf("%s (%s) %s", Version, CommitID, ExternalReleaseVersion)
}
