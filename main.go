package main

import (
	"flag"
	"log"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"net/http"

	"github.com/tarm/serial"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	conf   = flag.String("conf", "", "Configuration file")
	all    allports
	config Config
)

func initialize() error {

	// Parsing config yaml file to struct
	err := config.ParseYaml(*conf)
	if err != nil {
		return err
	}

	// redirect stdErr to stacktrace files
	fname := config.Logs.Inlogs + "stacktrace-" + strconv.Itoa(os.Getpid())
	f, err := os.Create(fname)
	if err != nil {
		log.Printf("Failed to open stackstrace file: %v", err)
		return err
	}
	err = syscall.Dup2(int(f.Fd()), int(os.Stderr.Fd()))
	if err != nil {
		log.Fatalf("Failed to redirect stderr to file: %v", err)
		return err
	}

	// Setting logger for agent logs
	logger := lumberjack.Logger{
		Filename:   config.Logs.Inlogs + "agent.log",
		MaxSize:    config.Logs.Maxsize,
		MaxBackups: config.Logs.Maxbackups,
		MaxAge:     config.Logs.Maxage,
	}
	log.SetOutput(&logger)

	// Fill the ports map with appropriate values from yaml config.
	all.ports = make(map[string]*serialport)
	for _, value := range config.Ports {
		all.ports[value.Name] = &serialport{name: value.Name, baudrate: value.Baudrate,
			comm: make(chan string, 1024), errorstatus: make(chan struct{})}
		all.ports[value.Name].infilelogger = &lumberjack.Logger{Filename: config.Logs.Inlogs + strings.Split(value.Name, "/")[2] + ".txt",
			MaxSize: config.Logs.Maxsize, MaxAge: config.Logs.Maxage, MaxBackups: config.Logs.Maxbackups}
		all.ports[value.Name].outfilelogger = &lumberjack.Logger{Filename: config.Logs.Outlogs + "out_" + strings.Split(value.Name, "/")[2],
			MaxSize: config.Logs.Maxsize, MaxAge: config.Logs.Maxage, MaxBackups: config.Logs.Maxbackups}
		all.ports[value.Name].clientactive.initialize()
	}

	// Open ports and start reader in separate go routine and will be blocked
	// into reader when port is not in error state or will blocked in port
	// opening state when port having error while opening.
	for name, value := range all.ports {
		go func(tmpname string, tmpvalue *serialport) {
			for {
				if all.ports[tmpname].port == nil {
					all.ports[tmpname].port, err = serial.OpenPort(&serial.Config{Name: tmpname,
						Baud: tmpvalue.baudrate})
					if err == nil {
						buf := make([]byte, 1024)
						for {
							number, err := all.ports[tmpname].port.Read(buf)
							if err != nil {
								log.Printf("Error receiving from stream %s\n", err)
								break
							}
							if buf == nil {
								log.Printf("Buf is nil.\n")
								break
							}
							tmpstring := string(buf[:number])
							_, _ = all.ports[tmpname].infilelogger.Write(buf[:number])
							select {
							case all.ports[tmpname].comm <- tmpstring:
							default:
							}
						}
						log.Printf("Error during reading port: %s\n", tmpname)
						all.ports[tmpname].port = nil
					} else {
						time.Sleep(10 * time.Second)
						log.Printf("Error opening port %s, will retry after 10Seconds.", tmpname)
					}
				} else {
					log.Printf("Port is already opened:%s", tmpname)
				}
			}
		}(name, value)
	}
	return nil
}

func main() {
	flag.Parse()
	if err := initialize(); err != nil {
		log.Fatalf("Error while initiliazing %s", err)
		return
	}
	r := createRouterRegisterPaths()
	err := http.ListenAndServe(":8083", r)
	if err != nil {
		log.Printf("net.http could not listen on address 8083: %s\n", err)
	}
}
