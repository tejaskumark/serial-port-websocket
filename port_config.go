package main

import (
	"errors"
	"strings"
	"sync"

	"go.bug.st/serial"
	"gopkg.in/natefinch/lumberjack.v2"
)

type serialport struct {
	mu           sync.Mutex
	port         serial.Port
	name         string
	baudrate     int
	status       uint8
	comm         chan string
	infilelogger *lumberjack.Logger
	// stop will be used by api request delete/edit/stop port.
	stop chan struct{}
	// ack will be returned by respective go routine on successful stop.
	ack          chan struct{}
	clientactive connection
}

type allports struct {
	mu    sync.Mutex
	ports map[string]*serialport
}

// jsonport struct
type jsonport struct {
	Newname  string `josn:"newname"`
	Desc     string `json:"description"`
	Baudrate int    `json:"baudrate"`
}

// addnewport will add new port with given info for add/edit operation.
func (p *allports) addnewport(pn string, pbr int, st uint8) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	for value := range p.ports {
		if p.ports[value].name == pn {
			return errors.New("port already exist")
		}
	}
	all.ports[pn] = &serialport{
		mu:       sync.Mutex{},
		port:     nil,
		name:     pn,
		baudrate: pbr,
		status:   st,
		comm:     make(chan string, 1024),
		infilelogger: &lumberjack.Logger{Filename: config.Logs.Inlogs + strings.Split(pn, "/")[2] + ".txt",
			MaxSize: config.Logs.Maxsize, MaxAge: config.Logs.Maxage, MaxBackups: config.Logs.Maxbackups},
		stop: make(chan struct{}, 1),
		ack:  make(chan struct{}),
		clientactive: connection{
			mu:         sync.Mutex{},
			connection: 0,
			raddr:      "",
		},
	}
	return nil
}

// initialize port will initialize default state for stop operation.
func (p *allports) initializeport(pn string, pbr int, st uint8) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	all.ports[pn] = &serialport{
		mu:       sync.Mutex{},
		port:     nil,
		name:     pn,
		baudrate: pbr,
		status:   st,
		comm:     make(chan string, 1024),
		infilelogger: &lumberjack.Logger{Filename: config.Logs.Inlogs + strings.Split(pn, "/")[2] + ".txt",
			MaxSize: config.Logs.Maxsize, MaxAge: config.Logs.Maxage, MaxBackups: config.Logs.Maxbackups},
		stop: make(chan struct{}, 1),
		ack:  make(chan struct{}),
		clientactive: connection{
			mu:         sync.Mutex{},
			connection: 0,
			raddr:      "",
		},
	}
	return nil
}

// removeElement will remove given portname from map and return suceess
// if element not found then return false
func (p *allports) removeElement(port string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	for name := range p.ports {
		if name == port {
			delete(p.ports, port)
			return true
		}
	}
	return false
}

// checkElement will check allports struct for given portname
// if found return true if found else false
func (p *allports) checkElement(port string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	for name := range p.ports {
		if name == port {
			return true
		}
	}
	return false
}

// getStatus will return status of port for a given port.
func (p *allports) getStatus(port string) (uint8, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, got := p.ports[port]; got {
		return p.ports[port].status, nil
	}
	return 2, errors.New("port not found")
}

// portStatusUpdate will return status of port for a given port.
func (p *allports) portStatusUpdate(port string, st uint8) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, got := p.ports[port]; got {
		p.ports[port].status = st
		return nil
	}
	return errors.New("port not found")
}
