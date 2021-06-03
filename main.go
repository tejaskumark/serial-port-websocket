package main

import (
	"flag"
	"log"
	"os"
	"strconv"
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

// Open ports and start reader in separate go routine and will be blocked
// into reader when port is not in error state or will blocked in port
// opening state when port having error while opening.
func initializereader(pn string) {
	go func(tmpname string) {
		if all.ports[tmpname].status == 1 {
			for {
				var err error
				if all.ports[tmpname].port == nil {
					all.ports[tmpname].port, err = serial.OpenPort(&serial.Config{Name: tmpname,
						Baud: all.ports[tmpname].baudrate})
					if err == nil {
						buf := make([]byte, 1024)
						for {
							select {
							case <-all.ports[tmpname].stop:
								log.Printf("Stopping mainreader for port:%s", tmpname)
								all.ports[tmpname].ack <- struct{}{}
								log.Printf("Stop ack send for mainreader port:%s", tmpname)
								return
							default:
								number, err := all.ports[tmpname].port.Read(buf)
								if err != nil {
									log.Printf("Error receiving from stream %s", err)
									all.ports[tmpname].port = nil
									break
								}
								if buf == nil {
									log.Printf("Buf is nil.")
									all.ports[tmpname].port = nil
									break
								}
								tmpstring := string(buf[:number])
								_, _ = all.ports[tmpname].infilelogger.Write(buf[:number])
								select {
								case all.ports[tmpname].comm <- tmpstring:
								default:
								}
							}
						}
					} else {
						select {
						case <-all.ports[tmpname].stop:
							log.Printf("Stopping mainreader for port:%s", tmpname)
							all.ports[tmpname].ack <- struct{}{}
							log.Printf("Stop ack send for mainreader port:%s", tmpname)
							return
						default:
							time.Sleep(10 * time.Second)
							log.Printf("Error: %s opening port %s, will retry after 10Seconds.",
								err, tmpname)
						}

					}
				}
			}
		} else {
			log.Printf("Port:%s status is disabled. Nothing to do.", tmpname)
		}
	}(pn)
}

func initialize() error {

	// Parsing config yaml file to struct
	err := config.parseYaml(*conf)
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
		all.addnewport(value.Name, value.Baudrate, value.Status)
	}

	for name := range all.ports {
		initializereader(name)
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
	errs := make(chan error)
	for _, value := range config.ServerConfig {
		if value.Enable == 1 {
			if value.Name == "http" {
				go func(port int) {
					log.Printf("http server starting")
					err := http.ListenAndServe(":"+strconv.Itoa(port), r)
					if err != nil {
						log.Printf("net.http could not listen: %s\n", err)
						errs <- err
					}
				}(value.Port)
			} else if value.Name == "https" {
				go func(port int, sslcert string, sslkey string) {
					log.Printf("https server starting")
					err := http.ListenAndServeTLS(":"+strconv.Itoa(port), sslcert, sslkey, r)
					if err != nil {
						log.Printf("net.https could not listen: %s", err)
						errs <- err
					}
				}(value.Port, value.SslCert, value.SslKey)
			} else {
				log.Printf("Unknown protocol %s... Skipping...", value.Name)
			}
		} else {
			log.Printf("Server disabled for protocol:%s", value.Name)
		}
	}
	<-errs
}
