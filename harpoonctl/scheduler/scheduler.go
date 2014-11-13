package scheduler

import (
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/codegangsta/cli"

	"github.com/soundcloud/harpoon/harpoonctl/log"
)

// Command is the scheduler subcommand.
var Command = cli.Command{
	Name:        "scheduler",
	Usage:       "Control a Harpoon agent",
	Description: "Interact with a Harpoon scheduler.",
	Subcommands: []cli.Command{registryCommand, psCommand, scheduleCommand, unscheduleCommand},
	Flags:       []cli.Flag{endpointFlag, schedulerFlag},
	Before:      parseEndpoint,
	HideHelp:    true,
}

var (
	schedulerPath    = filepath.Join(os.Getenv("HOME"), ".harpoonctl", "scheduler")
	defaultScheduler = filepath.Join(schedulerPath, "default")
)

var endpointFlag = cli.StringFlag{
	Name:  "e, endpoint",
	Value: "",
	Usage: "scheduler endpoint (overrides --scheduler)",
}

var schedulerFlag = cli.StringFlag{
	Name:  "s, scheduler",
	Value: "default",
	Usage: "read scheduler endpoint from " + schedulerPath + "/default",
}

var endpoint *url.URL

func parseEndpoint(c *cli.Context) error {
	// By default, connect to the scheduler on localhost.
	var ep = "http://localhost:4444"

	// Next, try to read the scheduler file.
	if buf, err := ioutil.ReadFile(filepath.Join(schedulerPath, c.String("scheduler"))); err == nil {
		ep = strings.TrimSpace(string(buf))
	}

	// Finally, if there is explicit --endpoint, it overrides everything else.
	if e := c.String("endpoint"); e != "" {
		ep = e
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
		u.Host = u.Host + ":4444"
	}

	log.Verbosef("using scheduler endpoint %s", u.String())
	endpoint = u

	return nil
}
