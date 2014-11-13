package scheduler

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/codegangsta/cli"

	"github.com/soundcloud/harpoon/harpoon-configstore/lib"
	schedulerapi "github.com/soundcloud/harpoon/harpoon-scheduler/api"
	"github.com/soundcloud/harpoon/harpoonctl/log"
)

var scheduleCommand = cli.Command{
	Name:        "schedule",
	ShortName:   "sched",
	Usage:       "schedule jobconfig.json",
	Description: "Schedules a job, as specified by jobconfig.json.",
	Action:      scheduleAction,
	Flags:       []cli.Flag{},
}

func scheduleAction(c *cli.Context) {
	filename := c.Args().First()
	if filename == "" {
		log.Fatalf("usage: schedule <jobconfig.json>")
	}

	buf, err := ioutil.ReadFile(filename)
	if err != nil {
		log.Fatalf("%s: %s", filename, err)
	}

	var cfg configstore.JobConfig
	if err := json.Unmarshal(buf, &cfg); err != nil {
		log.Fatalf("%s: %s", filename, err)
	}

	if err := cfg.Valid(); err != nil {
		log.Fatalf("%s: %s", filename, err)
	}

	req, err := http.NewRequest(
		"PUT",
		endpoint.String()+schedulerapi.APIVersionPrefix+schedulerapi.APISchedulePath,
		bytes.NewReader(buf),
	)
	if err != nil {
		log.Fatalf("%s", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatalf("%s: %s", endpoint.Host, err)
	}
	defer resp.Body.Close()

	var r schedulerapi.Response
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		log.Warnf("%s: when parsing response: %s", endpoint.Host, err)
	}

	log.Printf("%s: %s - %s", endpoint.Host, http.StatusText(r.StatusCode), r.Message)
}
