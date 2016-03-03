package main

import (
	"fmt"
	"sync"
)

type PacLoader struct {
	Pac               *Pac
	LocatorCollection *PacLocatorCollection
	mutex             *sync.RWMutex
	locatorChan       chan PacLocator
	stopChan          chan struct{}
	lastUsedLocator   PacLocator
}

func NewPacLoader(pac *Pac, locatorCollection *PacLocatorCollection) *PacLoader {
	return &PacLoader{
		Pac:               pac,
		LocatorCollection: locatorCollection,
		mutex:             &sync.RWMutex{},
	}
}

func (l *PacLoader) StartLocating() {
	l.StopLocating()
	l.mutex.Lock()
	defer l.mutex.Unlock()
	if l.stopChan == nil {
		l.stopChan = make(chan struct{})
	}
	if l.locatorChan == nil {
		l.locatorChan = make(chan PacLocator, 10)
	}
	go func() {
		for _, locator := range l.LocatorCollection.Locators {
			locator.StartLocating(l.locatorChan)
		}
		select {
		case <-l.stopChan:
			for _, locator := range l.LocatorCollection.Locators {
				locator.StopLocating()
			}
			l.mutex.Lock()
			defer l.mutex.Unlock()
			l.stopChan = nil
		}
	}()
	go func() {
		for {
			select {
			case locator := <-l.locatorChan:
				if locator == l.lastUsedLocator && locator.IsLocatedPacFileUpStillAvailable() {
					l.Load(locator.UUID())
				}
			}
		}
	}()
}

func (l *PacLoader) StopLocating() {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	if l.stopChan != nil {
		close(l.stopChan)
	}
}

func (l *PacLoader) Load(uuid UUID) error {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	locator, ok := l.LocatorCollection.Locators[uuid]
	if !ok {
		return fmt.Errorf("Unknown locator for %q", uuid.String())
	}
	if l.lastUsedLocator == nil {
		// Always populate last used loader if its currently nil
		l.lastUsedLocator = locator
	}
	data, err := locator.PacData()
	if err != nil {
		return err
	}
	err = l.Pac.LoadFrom(data, locator.PacURI(), locator.Description())
	if err == nil {
		// All good so update last used locator
		l.lastUsedLocator = locator
	}
	return err
}
