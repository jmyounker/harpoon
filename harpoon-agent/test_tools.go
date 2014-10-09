package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
)

var (
	// Chosen so they don't collide with the production range.
	lowTestPort  = uint16(rand.Intn((23000-100)-20000) + 20000)
	highTestPort = lowTestPort + 100
)

// getRandomUDPPort identifies a UDP port by attempting to connect to port zero. This
// port may-or-may-not-be available when you attempt to use it, but it's better than
// nothing.
func getRandomUDPPort() (int, error) {
	laddr, err := net.ResolveUDPAddr("udp", ":0")
	if err != nil {
		return 0, err
	}

	ln, err := net.ListenUDP("udp", laddr)
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.LocalAddr().(*net.UDPAddr).Port, nil
}

func dumpJSONPretty(v interface{}) string {
	x, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("CONVERSTION ERROR: %s", err)
	}
	return string(x)
}
