package scheduler

import (
	"bytes"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"

	"github.com/codegangsta/cli"

	schedulerapi "github.com/soundcloud/harpoon/harpoon-scheduler/api"
	"github.com/soundcloud/harpoon/harpoonctl/log"
)

var unscheduleCommand = cli.Command{
	Name:        "unschedule",
	ShortName:   "unsched",
	Usage:       "unschedule <jobconfig.json / hash>",
	Description: "Unschedules a job, as specified by jobconfig.json or hash.",
	Action:      unscheduleAction,
	Flags:       []cli.Flag{},
}

func unscheduleAction(c *cli.Context) {
	arg := c.Args().First()
	if arg == "" {
		log.Fatalf("usage: unschedule <jobconfig.json / hash>")
	}

	var (
		path string
		body io.Reader
	)

	buf, err := ioutil.ReadFile(arg)
	if err == nil {
		log.Verbosef("passing contents of %s as request body", arg)
		path = endpoint.String() + schedulerapi.APIVersionPrefix + schedulerapi.APIUnschedulePath
		body = bytes.NewReader(buf)
	} else {
		log.Verbosef("passing %s as job config hash", arg)
		path = endpoint.String() + schedulerapi.APIVersionPrefix + schedulerapi.APIUnschedulePath + "/" + arg
	}

	req, err := http.NewRequest("PUT", path, body)
	if err != nil {
		log.Fatalf("%s", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatalf("%s: %s", endpoint.Host, err)
	}

	var r schedulerapi.Response
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		log.Warnf("%s: when parsing response: %s", endpoint.Host, err)
	}

	log.Printf("%s: %s - %s", endpoint.Host, http.StatusText(r.StatusCode), r.Message)
}
