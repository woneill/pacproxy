// +build !plan9

package main

import (
	"os"
	"os/signal"
	"syscall"
)

func initSignalNotify(pac *Pac, locator PacLocator) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGHUP)
	go func() {
		for s := range sigChan {
			switch s {
			case syscall.SIGHUP:
				locator.Reload()
				pac.ConnService.Clear()
			}
		}
	}()
}
