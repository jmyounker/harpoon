package main

import (
	"fmt"
	"os"
	"runtime"
)

func systemHostname() string {
	name, err := os.Hostname()
	if err != nil {
		panic(err)
	}

	return name
}

func systemCPU() float64 {
	return float64(runtime.NumCPU())
}

func systemMem() int64 {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		panic(err)
	}
	defer f.Close()

	var kb uint64
	if _, err := fmt.Fscanf(f, "MemTotal: %d kB", &kb); err != nil {
		panic(err)
	}

	return int64(kb / 1024)
}
