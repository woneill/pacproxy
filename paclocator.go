package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"time"
)

// PacLocator provides an interface for being able to locate a pac file
type PacLocator interface {
	// Locate the file and send on the chan when located and/or updated.
	// Any sending on the chan must be non-blocking
	Locate() <-chan struct{}
	// LoadInto a pac instance the last located pac configuration
	LoadInto(pac *Pac)
}

// NewFilePacLocator for watching the local file system
func NewFilePacLocator(file string) *FilePacLocator {
	return &FilePacLocator{
		file: file,
	}
}

// FilePacLocator proviced a locator for a local file
type FilePacLocator struct {
	file    string
	modTime time.Time
	data    []byte
}

// Locate the file and send on the chan when located and/or updated
func (l *FilePacLocator) Locate() <-chan struct{} {
	ch := make(chan struct{})
	go func() {
		var delay time.Duration
		for {
			fInfo, err := os.Stat(l.file)
			if err != nil {
				log.Printf("Failed to located pac update at file://%s. Reason: %s", l.file, err)
				delay = time.Second * 10
			} else {
				delay = time.Second * 60
				if !fInfo.ModTime().Equal(l.modTime) {
					l.modTime = fInfo.ModTime()
					l.data, _ = ioutil.ReadFile(l.file)
					log.Printf("Located pac update at file://%s", l.file)
					select {
					case ch <- struct{}{}:
					default: // Non blocking!
					}
				} else {
					log.Printf("No change to pac at file://%s", l.file)
				}
			}
			timer := time.NewTimer(delay)
			select {
			case <-timer.C:
			}
		}
	}()
	return ch
}

// LoadInto a pac instance the last located pac configuration
func (l *FilePacLocator) LoadInto(pac *Pac) {
	pac.LoadFrom(l.data, fmt.Sprintf("file://%s", l.file), l.file)
}

// Stop the locator and close any open Locate() chan
func (l *FilePacLocator) Stop() {

}
