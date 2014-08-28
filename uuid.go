package main

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"time"
)

func uuid() string {
	var b [8]byte // 4 bytes time, 4 bytes rand

	binary.BigEndian.PutUint32(b[:], uint32(time.Now().Unix()))

	if _, err := rand.Read(b[4:]); err != nil {
		panic("unable to generate random id: " + err.Error())
	}

	return fmt.Sprintf("%x", b[:])
}
