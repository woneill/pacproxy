package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"sync"
	"time"
)

// PacLocator provides an interface for being able to locate a pac file
type PacLocator interface {
	// StartLocating the pac file and send on the chan when located and/or updated.
	// Any sending on the chan MUST be non-blocking. Send true if located, false if
	// locating failed.
	// Once located impleentations MUST load the contents of the pac into memory so
	// that it's available even if the source later becomes unreachable.
	// Implementations should continue to look/poll for updates to the pac files
	// until StopLocating is called.
	StartLocating(ch chan<- bool)
	// StopLocating the pac file. Any data for already located pac files should be
	// retained.
	StopLocating()
	// HasLocatedPacFile returns true if a pac file has, at any point, been located.
	// Please note that the pac file might not be up-to-date or still available.
	HasLocatedPacFile() bool
	// IsLocatedPacFileUpStillAvailable returns true if the located pac file is
	// still available. This might return false in a situation such as being able
	// to locate the file on disk one minute and then having the file deleted.
	IsLocatedPacFileUpStillAvailable() bool
	// PacFileWasLastLocatedAt returns the time for the last located pac file.
	PacFileWasLastLocatedAt() time.Time
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

/*
// Locate the pac file and send on the chan when located and/or updated.
func (l *CompositePacLocator) StartLocating(ch chan<- struct{}) {
	ch := make(chan struct{})
	for _, locator := range l.locator {
		go func(locator PacLocator) {
			lch := locator.StartLocating(ch)
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
*/

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
	mutex            *sync.RWMutex
	file             string
	data             []byte
	delay            time.Duration
	reloadChan       chan struct{}
	stopChan         chan struct{}
	isStillAvailable bool
	lastLocatedAt    time.Time
}

func (l *FilePacLocator) resetForStop() {
	l.mutex.Lock()
	l.stopChan = nil
	l.mutex.Unlock()
}

func (l *FilePacLocator) resetForReload() {
	l.mutex.Lock()
	l.data = []byte{}
	l.reloadChan = nil
	l.isStillAvailable = false
	l.lastLocatedAt = time.Time{}
	l.mutex.Unlock()
}

// StartLocating the pac file and send on the chan when located and/or updated.
// Any sending on the chan MUST be non-blocking.
func (l *FilePacLocator) StartLocating(ch chan<- bool) {
	l.StopLocating()
	go func() {
		sendToChan := false
		var lastFileInfo os.FileInfo
		var lastFileInfoErr error
		for {
			var timer *time.Timer
			l.mutex.Lock()
			if l.stopChan == nil {
				l.stopChan = make(chan struct{})
			}
			if l.reloadChan == nil {
				l.reloadChan = make(chan struct{})
			}
			l.mutex.Unlock()
			timer = time.NewTimer(l.delay)
			sendToChan = false
			fInfo, err := os.Stat(l.file)
			if err != nil {
				l.mutex.Lock()
				l.isStillAvailable = false
				l.mutex.Unlock()
				if lastFileInfoErr == nil {
					log.Printf("Failed to locate pac update at file://%s. Reason: %s", l.file, err)
					sendToChan = true
				}
			} else {
				l.mutex.Lock()
				l.isStillAvailable = true
				l.lastLocatedAt = time.Now()
				l.mutex.Unlock()
				if lastFileInfo == nil || !fInfo.ModTime().Equal(lastFileInfo.ModTime()) {
					sendToChan = true
					l.mutex.Lock()
					l.data, _ = ioutil.ReadFile(l.file)
					l.mutex.Unlock()
					log.Printf("Located pac update at file://%s", l.file)
				}
			}
			lastFileInfo = fInfo
			lastFileInfoErr = err
			// Non-Blocking select
			l.mutex.RLock()
			if sendToChan {
				select {
				case ch <- l.isStillAvailable:
				default:
				}
			}
			l.mutex.RUnlock()
			// Blocking select
			select {
			case <-timer.C:
			case <-l.reloadChan:
				timer = nil
				l.resetForReload()
			case <-l.stopChan:
				timer = nil
				l.resetForStop()
				return
			}
		}
	}()
}

// StopLocating the pac file
func (l *FilePacLocator) StopLocating() {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	if l.stopChan != nil {
		close(l.stopChan)
	}
}

// HasLocatedPacFile returns true if a pac file is available.
// Please note that this file might not be up-to-date or still available
func (l *FilePacLocator) HasLocatedPacFile() bool {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	return len(l.data) > 0
}

// IsLocatedPacFileUpStillAvailable returns true if the located pac file is
// still available. This might return false in a situation such as being able
// to locate the file on disk one minute and then having the file deleted.
func (l *FilePacLocator) IsLocatedPacFileUpStillAvailable() bool {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	return l.isStillAvailable
}

// PacFileWasLastLocatedAt returns the time for the last located pac file.
func (l *FilePacLocator) PacFileWasLastLocatedAt() time.Time {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	return l.lastLocatedAt
}

// LoadInto a pac instance the last located pac configuration
func (l *FilePacLocator) LoadInto(pac *Pac) error {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	if !l.HasLocatedPacFile() {
		msg := fmt.Sprintf("Unable to load file://%s as no data has yet been located", l.file)
		log.Println(msg)
		return errors.New(msg)
	}
	return pac.LoadFrom(l.data, fmt.Sprintf("file://%s", l.file), l.file)
}

// Reload a locator and force a location refresh
func (l *FilePacLocator) Reload() {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	close(l.reloadChan)
}
