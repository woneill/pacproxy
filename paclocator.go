package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"sync"
	"time"
)

// PacLocator provides an interface for being able to locate a pac file
type PacLocator interface {
	// Locate the pac file and send on the chan when located and/or updated.
	// Any sending on the chan should be blocking but you are encouraged
	// to use something like a timer to allow for polling and updating
	// the information thats being sent.
	Locate() <-chan struct{}
	// LoadInto a pac instance the last located pac configuration
	LoadInto(pac *Pac) error
	// Reload a locator and force a location refresh
	Reload()
}

// NewCompositePacLocator for a collection of locators
func NewCompositePacLocator(locator ...PacLocator) *CompositePacLocator {
	return &CompositePacLocator{
		mutex:   &sync.RWMutex{},
		locator: locator,
	}
}

// CompositePacLocator supports locating from a number of locators
type CompositePacLocator struct {
	mutex      *sync.RWMutex
	locator    []PacLocator
	lastLocate PacLocator
}

// Locate the pac file and send on the chan when located and/or updated.
func (l *CompositePacLocator) Locate() <-chan struct{} {
	ch := make(chan struct{})
	for _, locator := range l.locator {
		go func(locator PacLocator) {
			lch := locator.Locate()
			for {
				select {
				case <-lch:
					l.mutex.Lock()
					l.lastLocate = locator
					l.mutex.Unlock()
					ch <- struct{}{}
				}
			}
		}(locator)
	}
	return ch
}

// LoadInto a pac instance the last located pac configuration
func (l *CompositePacLocator) LoadInto(pac *Pac) error {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	if l.lastLocate == nil {
		return fmt.Errorf("Pac data has not yet been located")
	}
	return l.lastLocate.LoadInto(pac)
}

// Reload a locator and force a location refresh
func (l *CompositePacLocator) Reload() {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	for _, locator := range l.locator {
		locator.Reload()
	}
}

// NewFilePacLocator for watching the local file system
func NewFilePacLocator(file string) *FilePacLocator {
	return &FilePacLocator{
		mutex:      &sync.RWMutex{},
		file:       file,
		delay:      time.Second * 10,
		reloadChan: make(chan struct{}),
	}
}

// FilePacLocator proviced a locator for a local file
type FilePacLocator struct {
	mutex      *sync.RWMutex
	file       string
	modTime    time.Time
	data       []byte
	delay      time.Duration
	reloadChan chan struct{}
}

// Locate the pac file and send on the chan when located and/or updated
func (l *FilePacLocator) Locate() <-chan struct{} {
	ch := make(chan struct{})
	go func() {
		for {
			var timer *time.Timer
			l.mutex.Lock()
			if l.reloadChan == nil {
				l.reloadChan = make(chan struct{})
			}
			l.mutex.Unlock()
			fInfo, err := os.Stat(l.file)
			timer = time.NewTimer(l.delay)
			if err != nil {
				log.Printf("Failed to located pac update at file://%s. Reason: %s", l.file, err)
			} else {
				if !fInfo.ModTime().Equal(l.modTime) {
					log.Printf("Located pac update at file://%s", l.file)
					select {
					case ch <- struct{}{}:
						l.mutex.Lock()
						l.data, _ = ioutil.ReadFile(l.file)
						l.modTime = fInfo.ModTime()
						l.mutex.Unlock()
					case <-timer.C:
						timer = nil
					case <-l.reloadChan:
						timer = nil
						l.resetForReload()
					}
				} else {
					log.Printf("No change to pac at file://%s", l.file)
				}
			}
			if timer != nil {
				select {
				case <-timer.C:
				case <-l.reloadChan:
					l.resetForReload()
				}
			}
		}
	}()
	return ch
}

func (l *FilePacLocator) resetForReload() {
	l.mutex.Lock()
	l.data = []byte{}
	l.modTime = time.Time{}
	l.reloadChan = nil
	l.mutex.Unlock()
}

// LoadInto a pac instance the last located pac configuration
func (l *FilePacLocator) LoadInto(pac *Pac) error {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	if len(l.data) < 1 {
		return fmt.Errorf("Data for file://%s has not yet been located.", l.file)
	}
	return pac.LoadFrom(l.data, fmt.Sprintf("file://%s", l.file), l.file)
}

// Reload a locator and force a location refresh
func (l *FilePacLocator) Reload() {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	close(l.reloadChan)
}
