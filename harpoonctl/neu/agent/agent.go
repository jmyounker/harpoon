package agent

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/codegangsta/cli"

	"github.com/soundcloud/harpoon/harpoonctl/neu/log"
)

// Command is the agent subcommand.
var Command = cli.Command{
	Name:        "agent",
	Usage:       "Control Harpoon agents",
	Description: "Interact with Harpoon agents directly.",
	Subcommands: []cli.Command{resourcesCommand, psCommand, dumpCommand, logCommand, createCommand, startCommand, stopCommand, destroyCommand},
	Flags:       []cli.Flag{endpointFlag, clusterFlag},
	Before:      parseEndpoints,
	HideHelp:    true,
}

var (
	clusterPath    = filepath.Join(os.Getenv("HOME"), ".harpoonctl", "cluster") // like pdsh: $HOME/.dsh/group/foo
	defaultCluster = filepath.Join(clusterPath, "default")
)

var endpointFlag = cli.StringSliceFlag{
	Name:   "e, endpoint",
	Value:  &cli.StringSlice{},
	Usage:  "agent endpoint(s) (repeatable, overrides --cluster)",
	EnvVar: "",
}

var clusterFlag = cli.StringFlag{
	Name:   "c, cluster",
	Value:  "default",
	Usage:  "read agent endpoint(s) from " + clusterPath + "/default",
	EnvVar: "",
}

var endpoints = []*url.URL{}

func parseEndpoints(c *cli.Context) error {
	// By default, connect to the agent on localhost.
	var endpointStrs = []string{"http://localhost:3333"}

	// Next, try to read the cluster file.
	if buf, err := ioutil.ReadFile(filepath.Join(clusterPath, c.String("cluster"))); err == nil {
		endpointStrs = parseClusterBuffer(buf)
	}

	// Finally, if there are explicit --endpoints, they override everything else.
	if e := c.StringSlice("endpoint"); len(e) > 0 {
		endpointStrs = e
	}

	for _, ep := range endpointStrs {
		if ep == "" {
			continue
		}

		// Allow users to leave out the scheme.
		if !strings.HasPrefix(ep, "http") {
			ep = "http://" + ep
		}

		u, err := url.Parse(ep)
		if err != nil {
			return fmt.Errorf("%q: %s", ep, err)
		}

		// url.Parse interprets raw strings as url.Path.
		// Reinterpret them as the url.Host.
		if u.Host == "" && u.Path != "" {
			u.Host = u.Path
			u.Path = ""
		}

		// Allow users to leave out the port.
		if toks := strings.Split(u.Host, ":"); len(toks) == 1 {
			u.Host = u.Host + ":3333"
		}

		log.Verbosef("using agent endpoint %s", u.String())
		endpoints = append(endpoints, u)
	}

	return nil
}

func parseClusterBuffer(buf []byte) []string {
	var endpointStrs = []string{}

	for _, line := range bytes.Split(buf, []byte("\n")) {
		line = bytes.TrimSpace(line)
		endpointStrs = append(endpointStrs, string(line))
	}

	return endpointStrs
}
