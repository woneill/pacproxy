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

// PacLocatorCollection is a basic structure for keeping a collection of PacLocators
type PacLocatorCollection struct {
	Locators map[UUID]PacLocator
}

// NewPacLocatorCollection from one or more PacLocators
func NewPacLocatorCollection(l ...PacLocator) *PacLocatorCollection {
	c := &PacLocatorCollection{
		Locators: make(map[UUID]PacLocator),
	}
	c.Add(l...)
	return c
}

// Add locators to the collection
func (c *PacLocatorCollection) Add(l ...PacLocator) {
	for _, locator := range l {
		c.Locators[locator.UUID()] = locator
	}
}

// PacLocator provides an interface for being able to locate a pac file
type PacLocator interface {
	// UUID for the pac locator instance
	UUID() UUID
	// Description provides a string itendifier to describe the PacLocator
	Description() string
	// StartLocating the pac file and send on the chan when located and/or updated.
	// Any sending on the chan MUST be non-blocking.
	//
	// Once located impleentations MUST load the contents of the pac into memory so
	// that it's available even if the source later becomes unreachable.
	// Implementations should continue to look/poll for updates to the pac files
	// until StopLocating is called.
	StartLocating(ch chan<- PacLocator)
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
	// PacURI returns the URI for the pac data. Can return an empty string if a URI
	// is not applicable.
	PacURI() string
	// PacData returns the last located pac data
	PacData() ([]byte, error)
	// Reload a locator and force a location refresh
	Reload()
}

// NewFilePacLocator for watching the local file system
func NewFilePacLocator(file string) *FilePacLocator {
	return &FilePacLocator{
		mutex:      &sync.RWMutex{},
		uuid:       NewUUID(),
		file:       file,
		delay:      time.Second * 10,
		reloadChan: make(chan struct{}),
	}
}

// FilePacLocator proviced a locator for a local file
type FilePacLocator struct {
	mutex            *sync.RWMutex
	uuid             UUID
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

// UUID for the pac locator instance
func (l *FilePacLocator) UUID() UUID {
	return l.uuid
}

// Description provides a string itendifier to describe the PacLocator
func (l *FilePacLocator) Description() string {
	return fmt.Sprintf("File Locator file://%s", l.file)
}

// StartLocating the pac file and send on the chan when located and/or updated.
// Any sending on the chan MUST be non-blocking.
func (l *FilePacLocator) StartLocating(ch chan<- PacLocator) {
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
			if sendToChan {
				select {
				case ch <- l:
				default:
				}
			}
			// Blocking select
			select {
			case <-timer.C:
			case <-l.reloadChan:
				timer = nil
				lastFileInfo = nil
				lastFileInfoErr = nil
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

// PacURI returns the URI for the pac data.
func (l *FilePacLocator) PacURI() string {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	return l.file
}

// PacData returns the last located pac data
func (l *FilePacLocator) PacData() ([]byte, error) {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	if !l.HasLocatedPacFile() {
		return nil, errors.New("No pac data has yet been located")
	}
	return l.data, nil
}

// Reload a locator and force a location refresh
func (l *FilePacLocator) Reload() {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	close(l.reloadChan)
}
