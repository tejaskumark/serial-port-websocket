package main

import (
	"sync"
)

type connection struct {
	mu         sync.Mutex
	connection int8
}

func (connect *connection) increment() {
	connect.mu.Lock()
	connect.connection = connect.connection + 1
	connect.mu.Unlock()
}

func (connect *connection) decrement() {
	connect.mu.Lock()
	connect.connection = connect.connection - 1
	connect.mu.Unlock()
}

func (connect *connection) initialize() {
	connect.mu.Lock()
	connect.connection = 0
	connect.mu.Unlock()
}
