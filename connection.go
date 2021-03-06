package main

import (
	"sync"
)

type connection struct {
	mu         sync.Mutex
	connection int
	raddr      string
}

func (connect *connection) increment(addr string) {
	connect.mu.Lock()
	connect.connection = connect.connection + 1
	connect.raddr = addr
	connect.mu.Unlock()
}

func (connect *connection) decrement() {
	connect.mu.Lock()
	connect.connection = connect.connection - 1
	connect.raddr = ""
	connect.mu.Unlock()
}

func (connect *connection) getconncount() int {
	connect.mu.Lock()
	defer connect.mu.Unlock()
	return connect.connection
}

func (connect *connection) getraaddr() string {
	connect.mu.Lock()
	defer connect.mu.Unlock()
	return connect.raddr
}
