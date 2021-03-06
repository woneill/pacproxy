// Copyright 2014 Jack Wakefield
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//package gopac
package main

import (
	"sync"

	"github.com/robertkrimen/otto"
)

type gopacRuntime struct {
	mutex *sync.Mutex
	vm    *otto.Otto
}

func newGopacRuntime() (*gopacRuntime, error) {
	rt := &gopacRuntime{
		mutex: &sync.Mutex{},
		vm:    otto.New(),
	}

	rt.vm.Set("isPlainHostName", rt.isPlainHostName)
	rt.vm.Set("dnsDomainIs", rt.dnsDomainIs)
	rt.vm.Set("localHostOrDomainIs", rt.localHostOrDomainIs)
	rt.vm.Set("isResolvable", rt.isResolvable)
	rt.vm.Set("isInNet", rt.isInNet)
	rt.vm.Set("dnsResolve", rt.dnsResolve)
	rt.vm.Set("myIpAddress", rt.myIpAddress)
	rt.vm.Set("dnsDomainLevels", rt.dnsDomainLevels)
	rt.vm.Set("shExpMatch", rt.shExpMatch)

	if _, err := rt.vm.Run(gopacJavascriptUtils); err != nil {
		return nil, err
	}

	return rt, nil
}

func (rt *gopacRuntime) run(content string) error {
	rt.mutex.Lock()
	defer rt.mutex.Unlock()
	if _, err := rt.vm.Run(content); err != nil {
		return err
	}

	return nil
}

func (rt *gopacRuntime) findProxyForURL(url, host string) (string, error) {
	rt.mutex.Lock()
	defer rt.mutex.Unlock()
	value, err := rt.vm.Call("FindProxyForURL", nil, url, host)

	if err != nil {
		return "", err
	}

	var proxy string

	if proxy, err = otto.Value.ToString(value); err != nil {
		return "", err
	}

	return proxy, nil
}

func (rt *gopacRuntime) isPlainHostName(call otto.FunctionCall) otto.Value {
	if host, err := call.Argument(0).ToString(); err == nil {
		if value, err := rt.vm.ToValue(gopacIsPlainHostName(host)); err == nil {
			return value
		}
	}

	return otto.Value{}
}

func (rt *gopacRuntime) dnsDomainIs(call otto.FunctionCall) otto.Value {
	if host, err := call.Argument(0).ToString(); err == nil {
		if domain, err := call.Argument(1).ToString(); err == nil {
			if value, err := rt.vm.ToValue(gopacDnsDomainIs(host, domain)); err == nil {
				return value
			}
		}
	}

	return otto.Value{}
}

func (rt *gopacRuntime) localHostOrDomainIs(call otto.FunctionCall) otto.Value {
	if host, err := call.Argument(0).ToString(); err == nil {
		if hostdom, err := call.Argument(1).ToString(); err == nil {
			if value, err := rt.vm.ToValue(gopacLocalHostOrDomainIs(host, hostdom)); err == nil {
				return value
			}
		}
	}

	return otto.Value{}
}

func (rt *gopacRuntime) isResolvable(call otto.FunctionCall) otto.Value {
	if host, err := call.Argument(0).ToString(); err == nil {
		if value, err := rt.vm.ToValue(gopacIsResolvable(host)); err == nil {
			return value
		}
	}

	return otto.Value{}
}

func (rt *gopacRuntime) isInNet(call otto.FunctionCall) otto.Value {
	if host, err := call.Argument(0).ToString(); err == nil {
		if pattern, err := call.Argument(1).ToString(); err == nil {
			if mask, err := call.Argument(2).ToString(); err == nil {
				if value, err := rt.vm.ToValue(gopacIsInNet(host, pattern, mask)); err == nil {
					return value
				}
			}
		}
	}

	return otto.Value{}
}

func (rt *gopacRuntime) dnsResolve(call otto.FunctionCall) otto.Value {
	if host, err := call.Argument(0).ToString(); err == nil {
		if value, err := rt.vm.ToValue(gopacDnsResolve(host)); err == nil {
			return value
		}
	}

	return otto.Value{}
}

func (rt *gopacRuntime) myIpAddress(call otto.FunctionCall) otto.Value {
	if value, err := rt.vm.ToValue(gopacMyIpAddress()); err == nil {
		return value
	}

	return otto.Value{}
}

func (rt *gopacRuntime) dnsDomainLevels(call otto.FunctionCall) otto.Value {
	if host, err := call.Argument(0).ToString(); err == nil {
		if value, err := rt.vm.ToValue(gopacDnsDomainLevels(host)); err == nil {
			return value
		}
	}

	return otto.Value{}
}

func (rt *gopacRuntime) shExpMatch(call otto.FunctionCall) otto.Value {
	if str, err := call.Argument(0).ToString(); err == nil {
		if shexp, err := call.Argument(1).ToString(); err == nil {
			if value, err := rt.vm.ToValue(gopacShExpMatch(str, shexp)); err == nil {
				return value
			}
		}
	}

	return otto.Value{}
}
