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

var unscheduleCommand = cli.Command{
	Name:        "unschedule",
	ShortName:   "unsched",
	Usage:       "unschedule <jobconfig.json / hash>",
	Description: "Unschedules a job, as specified by jobconfig.json or hash.",
	Action:      unscheduleAction,
}

func unscheduleAction(c *cli.Context) {
	arg := c.Args().First()
	if arg == "" {
		log.Fatalf("usage: unschedule <jobconfig.json / hash>")
	}

	buf, err := ioutil.ReadFile(arg)
	if err == nil {
		log.Verbosef("interpreting %s as a config file", arg)

		var cfg configstore.JobConfig
		if err := json.Unmarshal(buf, &cfg); err != nil {
			log.Fatalf("%s: %s", arg, err)
		}

		if err := unscheduleConfig(cfg); err != nil {
			log.Fatalf("%s: %s", arg, err)
		}
	} else {
		log.Verbosef("interpreting %s as job config hash", arg)

		if err := unscheduleHash(arg); err != nil {
			log.Fatalf("%s: %s", arg, err)
		}
	}
}

func unscheduleConfig(cfg configstore.JobConfig) error {
	if err := cfg.Valid(); err != nil {
		return err
	}

	buf, err := json.Marshal(cfg)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(
		"PUT",
		endpoint.String()+schedulerapi.APIVersionPrefix+schedulerapi.APIUnschedulePath,
		bytes.NewReader(buf),
	)
	if err != nil {
		return err
	}

	return unscheduleRequest(req)
}

func unscheduleHash(jobConfigHash string) error {
	req, err := http.NewRequest(
		"PUT",
		endpoint.String()+schedulerapi.APIVersionPrefix+schedulerapi.APIUnschedulePath+"/"+jobConfigHash,
		nil,
	)
	if err != nil {
		log.Fatalf("%s", err)
	}

	return unscheduleRequest(req)
}

func unscheduleRequest(req *http.Request) error {
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var r schedulerapi.Response
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return err
	}

	log.Printf("%s: %s - %s", endpoint.Host, http.StatusText(r.StatusCode), r.Message)

	return nil
}
