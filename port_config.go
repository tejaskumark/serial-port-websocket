package main

import (
	"github.com/tarm/serial"
	"gopkg.in/natefinch/lumberjack.v2"
)

type serialport struct {
	port     *serial.Port
	name     string
	baudrate int
	status   string
	comm     chan string
	infilelogger *lumberjack.Logger
	outfilelogger *lumberjack.Logger
	errorstatus chan struct{}
	clientactive connection
}

type allports struct {
	ports map[string]*serialport
}