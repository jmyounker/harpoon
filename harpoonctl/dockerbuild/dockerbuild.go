package dockerbuild

import (
	"fmt"

	"github.com/codegangsta/cli"
)

// Command is the dockerbuild subcommand.
var Command = cli.Command{
	Name:        "dockerbuild",
	Usage:       "Build container from Dockerfile",
	Description: "Use the Docker build system to produce a Harpoon container.",
	Action:      buildAction,
	Flags: []cli.Flag{
		contextFlag,
		fromFlag,
		addFlag,
		imageFlag,
		tagFlag,
		outputFlag,
	},
	HideHelp: true,
}

var contextFlag = cli.StringFlag{
	Name:  "context",
	Value: "",
	Usage: "Docker build context, i.e. directory. Overrides other build options.",
}

var fromFlag = cli.StringFlag{
	Name:  "from",
	Value: defaultFrom,
	Usage: "Docker image to base your container on",
}

var addFlag = cli.StringSliceFlag{
	Name:  "add",
	Value: &cli.StringSlice{},
	Usage: "SRC:DST, file(s) to include in your container (ADD src dst) [repeatable]",
}

var imageFlag = cli.StringFlag{
	Name:  "image",
	Value: defaultImage,
	Usage: "Image name for built container",
}

var tagFlag = cli.StringFlag{
	Name:  "tag",
	Value: defaultTag,
	Usage: "Tag for built container (optional)",
}

var outputFlag = cli.StringFlag{
	Name:  "output",
	Value: defaultOutput,
	Usage: "Output filename, or stdout",
}

var (
	defaultFrom   = "FROM"
	defaultImage  = "IMAGE"
	defaultTag    = "TAG"
	defaultOutput = fmt.Sprintf("%s-%s-%s.tar.gz", defaultFrom, defaultImage, defaultTag)
)

func buildAction(ctx *cli.Context) {
	if contextFlag.Value != "" {
		buildContext(ctx, contextFlag.Value)
	} else {
		buildManual(ctx, fromFlag.Value, addFlag.Value.Value(), imageFlag.Value, tagFlag.Value, outputFlag.Value)
	}
}

func buildContext(ctx *cli.Context, context string) {

}

func buildManual(ctx *cli.Context, from string, add []string, outImage, outTag, outFile string) {

}
