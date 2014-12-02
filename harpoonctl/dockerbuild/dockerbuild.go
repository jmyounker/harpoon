package dockerbuild

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/codegangsta/cli"
	"github.com/ghemawat/stream"

	"github.com/soundcloud/harpoon/harpoonctl/log"
)

// Command is the dockerbuild subcommand.
var Command = cli.Command{
	Name:        "dockerbuild",
	Usage:       "Build container from Dockerfile",
	Description: "Use the Docker build system to produce a Harpoon container.",
	Action:      buildAction,
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "context",
			Usage: "Docker build context, i.e. directory. Overrides other build options.",
		},
		cli.StringFlag{
			Name:  "from",
			Value: defaultFrom,
			Usage: "Docker image to base your container on",
		},
		cli.StringSliceFlag{
			Name:  "add",
			Usage: "SRC:DST, file(s) to include in your container (ADD src dst) [repeatable]",
		},
		cli.StringFlag{
			Name:  "image",
			Value: defaultImage,
			Usage: "Name for built image",
		},
		cli.StringFlag{
			Name:  "tag",
			Value: defaultTag,
			Usage: "Tag for built container",
		},
		cli.StringFlag{
			Name:  "output",
			Value: defaultOutput,
			Usage: "Output filename, or stdout",
		},
	},
	HideHelp: true,
}

var (
	defaultFrom   = "FROM"
	defaultImage  = "IMAGE"
	defaultTag    = "TAG"
	defaultOutput = fmt.Sprintf("%s-%s-%s.tar.gz", defaultFrom, defaultImage, defaultTag)
	dockerPath    = ""
	dockerEnv     = os.Environ()
)

func buildAction(c *cli.Context) {
	if err := checkFlags(c); err != nil {
		log.Fatalf("%s", err)
	}

	if err := setGlobalDockerEnv(); err != nil {
		log.Fatalf("%s", err)
	}

	var err error
	if c.String("context") != "" {
		err = buildContext(c.String("context"), c.String("image"), c.String("tag"), c.String("output"))
	} else {
		err = buildManual(c.String("from"), c.StringSlice("add"), c.String("image"), c.String("tag"), c.String("output"))
	}

	if err != nil {
		log.Fatalf("%s", err)
	}
}

func buildManual(from string, add []string, image, tag, file string) error {
	contextPath := fmt.Sprintf(".harpoonctl-dockerbuild-%d", time.Now().UTC().UnixNano())
	if err := os.MkdirAll(contextPath, 0775); err != nil {
		return fmt.Errorf("when making temporary context directory: %s", err)
	}

	defer os.RemoveAll(contextPath)

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "FROM %s", from)
	for _, pair := range add {
		srcdst := strings.SplitN(pair, ":", 2)
		src, dst := srcdst[0], srcdst[1]
		if src == "" || dst == "" {
			return fmt.Errorf("--add %q invalid", add)
		}
		fmt.Fprintf(&buf, "ADD %s %s", src, dst)
	}

	if err := ioutil.WriteFile(filepath.Join(contextPath, "Dockerfile"), buf.Bytes(), 0775); err != nil {
		return fmt.Errorf("when writing Dockerfile: %s", err)
	}

	return buildContext(contextPath, image, tag, file)
}

func buildContext(contextPath, image, tag, file string) error {
	args := []string{dockerPath, "build"}
	if tag != defaultTag {
		args = append(args, "-t", tag)
	}
	args = append(args, contextPath)

	cmd := exec.Cmd{
		Path:   dockerPath,
		Args:   args,
		Env:    dockerEnv,
		Stdout: verboseWriter{},
		Stderr: errorWriter{},
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker build: %s", err)
	}

	return nil
}

func checkFlags(c *cli.Context) error {
	if contextPath := c.String("context"); contextPath != "" {
		if fi, err := os.Stat(contextPath); err != nil {
			return fmt.Errorf("--context %q: %s", contextPath, err)
		} else if !fi.IsDir() {
			return fmt.Errorf("--context %q: not a directory", contextPath)
		}
	} else {
		if c.String("from") == defaultFrom {
			return fmt.Errorf("must specify --from")
		}
		if c.String("image") == defaultImage {
			return fmt.Errorf("must specify --image")
		}
		if c.String("tag") == defaultTag {
			return fmt.Errorf("must specify --tag")
		}
	}

	return nil
}

func setGlobalDockerEnv() error {
	var err error

	dockerPath, err = exec.LookPath("docker")
	if err != nil {
		return fmt.Errorf("docker not found in $PATH")
	}

	log.Verbosef("docker found at %s", dockerPath)

	if runtime.GOOS == "darwin" {
		boot2dockerPath, err := exec.LookPath("boot2docker")
		if err != nil {
			return fmt.Errorf("boot2docker not found in $PATH")
		}

		log.Verbosef("boot2docker found at %s", boot2dockerPath)

		// Make sure VM is running.
		var buf bytes.Buffer
		if err := stream.Run(
			stream.Command("boot2docker", "status"),
			stream.WriteLines(&buf),
		); err != nil {
			return fmt.Errorf("boot2docker status: %s", err)
		}

		if status := string(bytes.TrimSpace(buf.Bytes())); status != "running" {
			return fmt.Errorf("boot2docker status: %s", status)
		}

		buf.Reset()

		// Get the relevant environment variables.
		if err := stream.Run(
			stream.Command("boot2docker", "shellinit"),
			stream.Grep("export"),
			stream.Substitute(`^[ ]*export ([^=]+)=(.*)$`, `$1=$2`),
			stream.WriteLines(&buf),
		); err != nil {
			return fmt.Errorf("boot2docker shellinit: %s", err)
		}

		for _, line := range strings.Split(buf.String(), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			dockerEnv = append(dockerEnv, line)
			log.Verbosef("%s", line)
		}
	}

	return nil
}

type verboseWriter struct{}

func (w verboseWriter) Write(p []byte) (int, error) {
	log.Verbosef("%s", bytes.TrimSpace(p))
	return len(p), nil
}

type errorWriter struct{}

func (w errorWriter) Write(p []byte) (int, error) {
	log.Errorf("%s", bytes.TrimSpace(p))
	return len(p), nil
}
