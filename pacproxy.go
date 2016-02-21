package main

//go:generate go-bindata-assetfs -pkg $GOPACKAGE -nomemcopy -nocompress -o bindata.go -prefix "resource/bindata/" resource/bindata/...
//go:generate gofmt -w bindata_assetfs.go

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
)

// Name of the application (also used when building packages)
const Name = "pacproxy"

// Version of the application (also used when building packages)
const Version = "1.1.0"

var (
	fPac     string
	fHelp    bool
	fListen  string
	fVerbose bool
	fVersion bool
)

func init() {
	flag.StringVar(&fPac, "c", "", "PAC file to use")
	flag.BoolVar(&fHelp, "h", false, "Print usage information")
	flag.StringVar(&fListen, "l", "127.0.0.1:8080", "Interface and port to listen on")
	flag.BoolVar(&fVerbose, "v", false, "Send verbose output to STDERR")
	flag.BoolVar(&fVersion, "version", false, fmt.Sprintf("Print the version (%s) and exit", Version))
}

func main() {
	flag.Parse()
	if fHelp {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		flag.PrintDefaults()
		os.Exit(2)
	}
	if fVersion {
		fmt.Fprintf(os.Stderr, "%s v%s\n", Name, Version)
		os.Exit(2)
	}
	if fVerbose {
		log.SetOutput(os.Stderr)
	} else {
		log.SetOutput(ioutil.Discard)
	}

	log.SetPrefix("")
	log.SetFlags(log.Ldate | log.Lmicroseconds)
	log.Printf("Starting %s v%s", Name, Version)

	pac, err := NewPac()
	if err != nil {
		log.Fatal(err)
	}
	if fPac != "" {
		err = pac.LoadFile(fPac)
		if err != nil {
			log.Fatal(err)
		}
	}

	initSignalNotify(pac)

	log.Printf("Listening on %q", fListen)
	log.Fatal(
		http.ListenAndServe(
			fListen,
			NewProxyHTTPHandler(pac, NewNonProxyHTTPHandler(pac)),
		),
	)
}
