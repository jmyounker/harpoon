package agent

import (
	"log"
	"os"
	"path/filepath"

	"github.com/codegangsta/cli"
)

var (
	clusterPath    = filepath.Join(os.Getenv("HOME"), ".harpoonctl", "cluster") // like pdsh: $HOME/.dsh/group/foo
	defaultCluster = filepath.Join(clusterPath, "default")
)

var Command = cli.Command{
	Name:        "agent",
	ShortName:   "a",
	Usage:       "Control Harpoon agents",
	Description: "Interact with Harpoon agents directly.",
	Subcommands: []cli.Command{resourcesCommand},
	Flags:       []cli.Flag{endpointFlag, clusterFlag},
	Before:      before,
}

var endpointFlag = cli.StringSliceFlag{
	Name:   "e, endpoint",
	Value:  &cli.StringSlice{},
	Usage:  "agent endpoint(s) (repeatable, overrides --cluster)",
	EnvVar: "",
}

var clusterFlag = cli.StringFlag{
	Name:   "c, cluster",
	Value:  "",
	Usage:  "read agent endpoint(s) from " + clusterPath + "/NAME",
	EnvVar: "",
}

func before(c *cli.Context) error {
	log.Printf("agent.Command GlobalStringSlice endpoint %v", c.GlobalStringSlice("endpoint"))
	log.Printf("agent.Command StringSlice endpoint %v", c.StringSlice("endpoint"))
	log.Printf("agent.Command GlobalStringSlice e %v", c.GlobalStringSlice("e"))
	log.Printf("agent.Command StringSlice e %v", c.StringSlice("e"))

	return nil
}
