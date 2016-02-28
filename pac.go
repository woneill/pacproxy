package main

import (
	"bytes"
	"errors"
	"expvar"
	"fmt"
	"io"
	"log"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/robertkrimen/otto"
)

const pacMaxStatements = 10

var (
	pacStatementSplit                    *regexp.Regexp
	pacItemSplit                         *regexp.Regexp
	pacCallFindProxyForURLResultCount    *expvar.Map
	pacCallFindProxyForURLParamHostCount *expvar.Map
)

func init() {
	pacStatementSplit = regexp.MustCompile(`\s*;\s*`)
	pacItemSplit = regexp.MustCompile(`\s+`)

	pacCallFindProxyForURLResultCount = new(expvar.Map).Init()
	pacCallFindProxyForURLParamHostCount = new(expvar.Map).Init()

	callFindProxyForURLMap := new(expvar.Map).Init()
	callFindProxyForURLMap.Set("resultCount", pacCallFindProxyForURLResultCount)
	callFindProxyForURLMap.Set("urlHostCount", pacCallFindProxyForURLParamHostCount)

	pacExpvarMap := expvar.NewMap("pac")
	pacExpvarMap.Set("callFindProxyForURL", callFindProxyForURLMap)

}

// Pac is the main proxy auto configuration engine.
type Pac struct {
	mutex       *sync.RWMutex
	runtime     *gopacRuntime
	pacURI      string
	pacSrc      []byte
	ConnService *PacConnService
}

// NewPac create a new pac instance.
func NewPac() (*Pac, error) {
	p := &Pac{
		mutex:       &sync.RWMutex{},
		ConnService: NewPacConnService(),
	}
	if err := p.Unload(); err != nil {
		return nil, err
	}
	return p, nil
}

// Unload any previously loaded pac configuration and reverts to default.
func (p *Pac) Unload() error {
	log.Print("Unloading pac")
	return p.LoadString(MustAsset("default.pac"))
}

// LoadFrom attempts to load a pac from a string, a byte slice,
// a bytes.Buffer, or an io.Reader, but it MUST always be in UTF-8.
// Allows specifying an extra identifier for logging to state where
// the pac was sourced from.
func (p *Pac) LoadFrom(js interface{}, uri string, from string) error {
	log.Printf("Loading pac from %s", from)
	err := p.initPacRuntime(js, uri)
	if err == nil {
		log.Print("PAC runtime successfully initialised")
		log.Print("Old PAC runtime has been replaced")
	} else {
		log.Printf("PAC runtime initialisation failed. Reason: %s", err)
		log.Print("Existing PAC runtime has not been replaced")
	}
	return err
}

// LoadString attempts to load a pac from a string, a byte slice,
// a bytes.Buffer, or an io.Reader, but it MUST always be in UTF-8.
func (p *Pac) LoadString(js interface{}) error {
	return p.LoadFrom(js, "", "string")
}

// LoadFile attempt to load a pac file.
func (p *Pac) LoadFile(file string) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	absFile, _ := filepath.Abs(f.Name())
	uri := fmt.Sprintf("file://%s", absFile)
	return p.LoadFrom(f, uri, fmt.Sprintf("file %s", absFile))
}

func (p *Pac) initPacRuntime(js interface{}, uri string) error {
	var err error
	var runtime *gopacRuntime
	runtime, err = newGopacRuntime()
	if err != nil {
		return err
	}
	formatForConsole := func(argumentList []otto.Value) string {
		output := []string{}
		for _, argument := range argumentList {
			output = append(output, fmt.Sprintf("%v", argument))
		}
		return strings.Join(output, " ")
	}
	runtime.vm.Set("alert", func(call otto.FunctionCall) otto.Value {
		log.Println("alert:", formatForConsole(call.ArgumentList))
		return otto.UndefinedValue()
	})
	runtime.vm.Set("console", map[string]interface{}{
		"assert": func(call otto.FunctionCall) otto.Value {
			if b, _ := call.Argument(0).ToBoolean(); !b {
				log.Println("console.assert:", formatForConsole(call.ArgumentList[1:]))
			}
			return otto.UndefinedValue()
		},
		"clear": func(call otto.FunctionCall) otto.Value {
			log.Println("console.clear: -------------------------------------")
			return otto.UndefinedValue()
		},
		"debug": func(call otto.FunctionCall) otto.Value {
			log.Println("console.debug:", formatForConsole(call.ArgumentList))
			return otto.UndefinedValue()
		},
		"error": func(call otto.FunctionCall) otto.Value {
			log.Println("console.error:", formatForConsole(call.ArgumentList))
			return otto.UndefinedValue()
		},
		"info": func(call otto.FunctionCall) otto.Value {
			log.Println("console.info:", formatForConsole(call.ArgumentList))
			return otto.UndefinedValue()
		},
		"log": func(call otto.FunctionCall) otto.Value {
			log.Println("console.log:", formatForConsole(call.ArgumentList))
			return otto.UndefinedValue()
		},
		"warn": func(call otto.FunctionCall) otto.Value {
			log.Println("console.warn:", formatForConsole(call.ArgumentList))
			return otto.UndefinedValue()
		},
	})
	var pacSrc []byte
	switch js := js.(type) {
	case string:
		pacSrc = []byte(js)
	case []byte:
		pacSrc = js
	case *bytes.Buffer:
		pacSrc = js.Bytes()
	case io.Reader:
		var buf bytes.Buffer
		io.Copy(&buf, js)
		pacSrc = buf.Bytes()
	default:
		return errors.New("invalid source")
	}
	if _, err := runtime.vm.Run(pacSrc); err != nil {
		return err
	}
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.pacURI = uri
	p.pacSrc = pacSrc
	p.runtime = runtime
	p.ConnService.Clear()
	return nil
}

// PacURI returns the URI of the currently loaded pac configuration.
// Returns an empty string is the pac configuration was not loaded from a URI.
func (p *Pac) PacURI() string {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return p.pacURI
}

// PacConfiguration will return the current pac configuration
func (p *Pac) PacConfiguration() []byte {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return p.pacSrc
}

// HostFromURL takes a URL and return the host as it would be passed
// to the FindProxtForURL host argument.
func (p *Pac) HostFromURL(in *url.URL) string {
	if o := strings.Index(in.Host, ":"); o >= 0 {
		return in.Host[:o]
	}
	return in.Host
}

// CallFindProxyForURLFromURL using the current pac for a *url.URL.
func (p *Pac) CallFindProxyForURLFromURL(in *url.URL) (string, error) {
	return p.CallFindProxyForURL(in.String(), p.HostFromURL(in))
}

// CallFindProxyForURL using the current pac.
func (p *Pac) CallFindProxyForURL(url, host string) (string, error) {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	pacCallFindProxyForURLParamHostCount.Add(host, 1)
	s, e := p.runtime.findProxyForURL(url, host)
	if s != "" {
		pacCallFindProxyForURLResultCount.Add(s, 1)
	}
	return s, e
}

// PacConn returns a *PacConn for the in *url.URL, processing
// the result of the pac find proxy result and trying to ensure that
// the proxy is active.
func (p *Pac) PacConn(in *url.URL) (*PacConn, error) {
	if in == nil {
		return nil, nil
	}
	urlStr := in.String()
	hostStr := p.HostFromURL(in)
	s, err := p.CallFindProxyForURL(urlStr, hostStr)
	if err != nil {
		log.Printf(
			"Failed to invoke FindProxyForURL(%q, %q). Reason: %s",
			urlStr,
			hostStr,
			err.Error(),
		)
		return nil, err
	}
	errMsg := bytes.NewBufferString(
		fmt.Sprintf(
			"Unable to process FindProxyForURL(%q, %q) result %q.",
			urlStr,
			hostStr,
			s,
		),
	)
	okMsg := bytes.NewBufferString(
		fmt.Sprintf(
			"Successfully processed FindProxyForURL(%q, %q) result %q.",
			urlStr,
			hostStr,
			s,
		),
	)
	for _, statement := range pacStatementSplit.Split(s, pacMaxStatements) {
		part := pacItemSplit.Split(statement, 2)
		switch strings.ToUpper(part[0]) {
		case "DIRECT":
			okMsg.Write([]byte(" Going DIRECT."))
			log.Print(okMsg.String())
			return nil, nil
		case "PROXY":
			pacConn := p.ConnService.Conn(part[1])
			if pacConn.IsActive() {
				fmt.Fprintf(okMsg, " Using PROXY %q.", part[1])
				log.Print(okMsg.String())
				return pacConn, nil
			}
			okMsg.Write([]byte(" "))
			okMsg.WriteString(pacConn.Error().Error())
			okMsg.Write([]byte("."))
			errMsg.Write([]byte(" "))
			errMsg.WriteString(pacConn.Error().Error())
			errMsg.Write([]byte("."))
		default:
			fmt.Fprintf(errMsg, " Unsupported PAC command %q.", part[0])
		}
	}
	errStr := errMsg.String()
	log.Print(errStr)
	return nil, errors.New(errStr)
}

// Proxy returns the URL of the proxy that the client should use.
// If the client should establish a direct connect that it will return
// nil. Can be easly wrapped for use in http.Transport.Proxy
func (p *Pac) Proxy(in *url.URL) (*url.URL, error) {
	pc, err := p.PacConn(in)
	if pc != nil {
		return url.Parse("http://" + pc.Address())
	}
	return nil, err
}

// Dial can be used for http.Transport.Dial and allows us to reuse
// a net.Conn that we might already have to a proxy server.
func (p *Pac) Dial(n, address string) (net.Conn, error) {
	if p.ConnService.IsKnownProxy(address) {
		return p.ConnService.Conn(address).Dial()
	}
	return net.Dial(n, address)
}
