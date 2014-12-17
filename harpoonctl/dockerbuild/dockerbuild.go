package dockerbuild

import (
	"bytes"
	"fmt"
	"io"
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
			Value: &cli.StringSlice{},
			Usage: "SRC:DST, file(s) to include in your container (ADD src dst) [repeatable]",
		},
		cli.StringFlag{
			Name:  "tag",
			Value: defaultTag,
			Usage: "Tag for built image",
		},
		cli.StringFlag{
			Name:  "output",
			Value: defaultOutput,
			Usage: "Output filename, or URL to POST",
		},
		cli.BoolFlag{
			Name:  "keep",
			Usage: "keep intermediate context directory",
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
		err = buildContext(c.String("context"), c.String("tag"), c.String("output"))
	} else {
		err = buildManual(c.String("from"), c.StringSlice("add"), c.String("tag"), c.String("output"), c.Bool("keep"))
	}

	if err != nil {
		log.Fatalf("%s", err)
	}
}

func buildManual(from string, add []string, tag, output string, keep bool) error {
	// Create the context directory
	contextPath := fmt.Sprintf(".harpoonctl-dockerbuild-%d", time.Now().UTC().UnixNano())
	if err := os.MkdirAll(contextPath, 0775); err != nil {
		return fmt.Errorf("when making temporary context directory: %s", err)
	}

	if !keep {
		defer os.RemoveAll(contextPath)
	}

	// Parse out all the --add files.
	type addTuple struct {
		srcAbs string
		srcRel string
		dst    string
	}

	var t []addTuple

	for _, pair := range add {
		srcdst := strings.SplitN(pair, ":", 2)
		if len(srcdst) != 2 {
			return fmt.Errorf("--add %q: invalid format", pair)
		}

		src, dst := srcdst[0], srcdst[1]

		srcAbs, err := filepath.Abs(src)
		if err != nil {
			return fmt.Errorf("--add %q: %s", pair, err)
		}

		if _, err := os.Stat(srcAbs); err != nil {
			return fmt.Errorf("--add %q: %s", pair, err)
		}

		srcRel := filepath.Base(srcAbs)
		t = append(t, addTuple{srcAbs, srcRel, dst})
	}

	// Copy each --add file to the context directory.
	for _, t := range t {
		var (
			src = t.srcAbs
			dst = filepath.Join(contextPath, t.srcRel)
		)

		log.Verbosef("cp %s %s", src, dst)

		if err := cp(src, dst); err != nil {
			return err
		}
	}

	// Build the Dockerfile.
	var (
		buf = bytes.Buffer{}
		w   = teeWriter{&buf, verboseWriter{}}
	)
	fmt.Fprintf(w, "FROM %s\n", from)
	for _, t := range t {
		fmt.Fprintf(w, "ADD %s %s\n", t.srcRel, t.dst)
	}

	if err := ioutil.WriteFile(filepath.Join(contextPath, "Dockerfile"), buf.Bytes(), 0775); err != nil {
		return fmt.Errorf("when writing Dockerfile: %s", err)
	}

	return buildContext(contextPath, tag, output)
}

func buildContext(contextPath, tag, output string) error {
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
		Stderr: verboseWriter{},
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker build: %s", err)
	}

	cmd = exec.Cmd{
		Path:   dockerPath,
		Args:   []string{dockerPath, "run", "--entrypoint", "echo", tag, "no-op"},
		Env:    dockerEnv,
		Stdout: verboseWriter{},
		Stderr: verboseWriter{},
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker run: %s", err)
	}

	cmd = exec.Cmd{
		Path: dockerPath,
		Args: []string{dockerPath, "ps", "--latest", "--quiet"},
		Env:  dockerEnv,
	}

	buf, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker ps --latest: %s", err)
	}

	containerID := strings.TrimSpace(string(buf))
	if containerID == "" {
		return fmt.Errorf("docker ps --latest: no output")
	}

	log.Verbosef("image %q → container ID %s", tag, containerID)

	return export(containerID, output)
}

func export(containerID, output string) error {
	if output == "" || output == defaultOutput {
		panic("invalid --output escaped first-pass checking")
	}

	var (
		chain    = []string{dockerPath, "export", containerID}
		compress = false
		upload   = false
	)

	if strings.HasSuffix(output, ".tar") {
		// OK
	} else if strings.HasSuffix(output, ".tar.gz") || strings.HasSuffix(output, ".tgz") {
		compress = true
	} else {
		return fmt.Errorf("only .tar, .tar.gz, and .tgz are supported")
	}

	if strings.HasPrefix(output, "http") {
		upload = true
	}

	if compress {
		chain = append(chain, "|", "gzip", "-9")
	}

	if upload {
		chain = append(chain, "|", "curl", "-Ss", "-XPOST", "--data-binary", "@-", output)
	} else {
		chain = append(chain, ">", output)
	}

	str := strings.Join(chain, " ")

	log.Verbosef("sh -c %q", str)

	shPath, err := exec.LookPath("sh")
	if err != nil {
		return fmt.Errorf("can't find sh: %s", err)
	}

	cmd := exec.Cmd{
		Path:   shPath,
		Args:   []string{shPath, "-c", str},
		Env:    dockerEnv,
		Stdout: verboseWriter{},
		Stderr: verboseWriter{},
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker export: %s", err)
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
		if c.String("tag") == defaultTag {
			return fmt.Errorf("must specify --tag")
		}
		if c.String("output") == defaultOutput {
			return fmt.Errorf("must specify --output")
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
			return fmt.Errorf("boot2docker status failed: %s", err)
		}

		if status := string(bytes.TrimSpace(buf.Bytes())); status != "running" {
			return fmt.Errorf("boot2docker status: %s (try: boot2docker up)", status)
		}

		buf.Reset()

		// Get the relevant environment variables.
		if err := stream.Run(
			stream.Command("boot2docker", "shellinit"),
			stream.Grep("export"),
			stream.Substitute(`^[ ]*export ([^=]+)=(.*)$`, `$1=$2`),
			stream.WriteLines(&buf),
		); err != nil {
			return fmt.Errorf("boot2docker shellinit failed: %s", err)
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

type teeWriter struct{ a, b io.Writer }

func (w teeWriter) Write(p []byte) (int, error) {
	n0, err0 := w.a.Write(p)
	if err0 != nil {
		return n0, err0
	}

	n1, err1 := w.b.Write(p)
	if err1 != nil {
		return n1, err1
	}

	if n0 != n1 {
		panic(fmt.Sprintf("teeWriter had irregular writes: %d != %d", n0, n1))
	}

	return n0, nil
}

func cp(srcFilename, dstFilename string) error {
	src, err := os.Open(srcFilename)
	if err != nil {
		return err
	}

	defer src.Close()

	dst, err := os.Create(dstFilename)
	if err != nil {
		return err
	}

	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return err
	}

	return nil
}
