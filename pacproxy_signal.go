// +build !plan9

package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
)

func initSignalNotify(pac *Pac, pacLocators *PacLocatorCollection) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGHUP)
	go func() {
		for s := range sigChan {
			switch s {
			case syscall.SIGHUP:
				log.Print("Caught SIGHUP")
				for _, locator := range pacLocators.Locators {
					locator.Reload()
				}
				pac.ConnService.Clear()
			}
		}
	}()
}
