package main

import (
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

var (
	// websocket upgrader
	upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
	// Binary directory path
	absPath string
	// websocket ping/poing timeout for connection check
	pongWait   = 30 * time.Second
	pingPeriod = (pongWait * 9) / 10
	writeWait  = 10 * time.Second
)

// registerPaths will register all paths at one go.
func registerPaths(r *mux.Router) {
	staticDir := "/ui/"
	r.PathPrefix(staticDir).Handler(http.StripPrefix(staticDir, http.FileServer(http.Dir(absPath+staticDir))))
	fs := http.FileServer(http.Dir(config.Logs.Inlogs))
	r.PathPrefix("/logs/").Handler(http.StripPrefix("/logs/", fileserve(fs)))
	r.HandleFunc("/serialconsole", all.webSocketHandler).Queries("portname", "{.*}")
	r.HandleFunc("/get/config", config.getConfig).Methods("GET")
	r.HandleFunc("/port", servePortHtml).Methods("GET")
	r.HandleFunc("/", serveHomeHtml).Methods("GET")
	r.HandleFunc("/delete", deletePort).Methods("DELETE").Queries("portname", "{.*}")
	r.HandleFunc("/edit", editPort).Methods("POST").Queries("portname", "{.*}")
	r.HandleFunc("/add", addPort).Methods("POST")
	r.HandleFunc("/stop", stopPort).Methods("POST").Queries("portname", "{.*}")
	r.HandleFunc("/start", startPort).Methods("POST").Queries("portname", "{.*}")
	r.HandleFunc("/getactivesession", getActiveSession).Methods("GET").Queries("portname", "{.*}")
}

// getActiveSession will return active session count on givne port
func getActiveSession(w http.ResponseWriter, r *http.Request) {
	var pname = r.FormValue("portname")
	if pname == "" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Portname is missing in reuqest."))
		return
	}

	// check if there are element with respect to such ports, if not return error.
	if !config.checkElement(pname) || !all.checkElement(pname) {
		log.Println(config.Ports)
		log.Println(all.ports)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Config/allport struct missing given port."))
		return
	}
	w.Write([]byte(strconv.Itoa(all.ports[pname].clientactive.connection)))
	return
}

// startPort will start serial port
func startPort(w http.ResponseWriter, r *http.Request) {
	var pname = r.FormValue("portname")
	msg, st := commonCheck(pname)
	if msg != "" {
		w.WriteHeader(st)
		w.Write([]byte(msg))
		return
	}
	all.ports[pname].clientactive.increment(r.RemoteAddr)
	defer all.ports[pname].clientactive.decrement()
	if st, _ := config.getStatus(pname); st == 1 {
		return
	}
	if st, _ := all.getStatus(pname); st == 1 {
		return
	}
	config.portStatusUpdate(pname, 1)
	all.portStatusUpdate(pname, 1)
	err := config.writeYaml(*conf)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Error writing YAML file."))
		log.Panicf("[Client:" + r.RemoteAddr + " Serial Port:" + "]" + pname + err.Error())
		return
	}
	initializereader(pname)
	return
}

// stopPort will stop serial port
func stopPort(w http.ResponseWriter, r *http.Request) {
	var pname = r.FormValue("portname")
	msg, st := commonCheck(pname)
	if msg != "" {
		w.WriteHeader(st)
		w.Write([]byte(msg))
		return
	}
	all.ports[pname].clientactive.increment(r.RemoteAddr)
	defer all.ports[pname].clientactive.decrement()
	if st, _ := config.getStatus(pname); st == 2 {
		return
	}
	if st, _ := all.getStatus(pname); st == 2 {
		return
	}
	_ = mainReaderClose(pname)
	log.Printf("[Client:%s Serial Port:%s]Main reader go routine closed.",
		r.RemoteAddr, pname)
	config.portStatusUpdate(pname, 2)
	all.initializeport(pname, all.ports[pname].baudrate, 2)
	err := config.writeYaml(*conf)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Error writing YAML file."))
		log.Panicf("[Client:" + r.RemoteAddr + " Serial Port:" + "]" + pname + err.Error())
		return
	}
	return
}

// mainReaderClose will close main reader go routine and return
func mainReaderClose(portname string) bool {
	for {
		select {
		case <-all.ports[portname].ack:
			log.Printf("[Port:%s]Mainreader go routine closed. Ack recived.", portname)
			return true
		default:
			all.ports[portname].stop <- struct{}{}
			log.Printf("[Port:%s]Mainreader go routine stop send.", portname)
			if all.ports[portname].port != nil {
				all.ports[portname].port.Write([]byte("golang\n"))
			}
			time.Sleep(200 * time.Millisecond)
		}
	}
}

// commonCheck function will check common condition for delete/edit request of API
// and return error string and respective http status code if any.
func commonCheck(pname string) (string, int) {
	if pname == "" {
		return "Portname is missing in reuqest.", http.StatusBadRequest
	}

	// check if there are element with respect to such ports, if not return error.
	if !config.checkElement(pname) || !all.checkElement(pname) {
		log.Println(config.Ports)
		log.Println(all.ports)
		return "Config/allport struct missing given port.", http.StatusInternalServerError
	}

	// check if there are any other existing session active or not.
	// and lock session on this port any more.
	if all.ports[pname].clientactive.connection >= 1 {
		msg := "Can not delete/edit/stop/start port .One session active with IP:" + all.ports[pname].clientactive.raddr + ".Try after sometime."
		return msg, http.StatusForbidden
	}
	return "", http.StatusOK
}

// jsondecoder will decode given request body for port edit and add
// return error if any element missing or extra element present.
func (p *jsonport) jsondecoder(r *http.Request) error {
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	err := d.Decode(&p)
	if err != nil {
		return err
	}
	return nil
}

// editPort will delete port configuration if portname or baudrate changing
// if only desc is changing then port deletion not required.
func editPort(w http.ResponseWriter, r *http.Request) {
	var pname = r.FormValue("portname")
	var jport jsonport
	err := jport.jsondecoder(r)
	if err != nil {
		log.Printf("[Client:%s Serial Port:%s]Error in posted JSON decode:%s",
			r.RemoteAddr, pname, err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	msg, status := commonCheck(pname)
	if msg != "" {
		w.WriteHeader(status)
		w.Write([]byte(msg))
		return
	}
	all.ports[pname].clientactive.increment(r.RemoteAddr)
	if jport.Baudrate == all.ports[pname].baudrate && jport.Newname == pname {
		err = config.updateElement(jport.Newname, jport.Desc)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			return
		}
		err = config.writeYaml(*conf)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Some error while writing YAML.Service going into panic..."))
			log.Panicf(err.Error())
		}
		all.ports[pname].clientactive.decrement()
		return
	} else {
		if st := all.checkElement(jport.Newname); st {
			all.ports[pname].clientactive.decrement()
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Provided port name already exist."))
			return
		}
		if st := config.checkElement(jport.Newname); st {
			all.ports[pname].clientactive.decrement()
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Provided port name already exist."))
			return
		}
		if all.ports[pname].status == 1 {
			mainReaderClose(pname)
			log.Printf("[Client:%s Serial Port:%s]Main reader go routine closed.",
				r.RemoteAddr, pname)
		}

		all.removeElement(pname)
		log.Printf("[Client:%s Serial Port:%s]Port removed from allports struct.",
			r.RemoteAddr, pname)

		config.removeElement(pname)
		log.Printf("[Client:%s Serial Port:%s]Port removed from config struct.",
			r.RemoteAddr, pname)

		err = all.addnewport(jport.Newname, jport.Baudrate, 1)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(err.Error()))
			return
		}
		err = config.addElement(jport.Newname, jport.Baudrate, jport.Desc)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(err.Error()))
			return
		}

		err = config.writeYaml(*conf)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Service going into panic. Start again..."))
			log.Panicf("[Client:" + r.RemoteAddr + " Serial Port:" + "]" + pname + err.Error())
		}
		log.Printf("[Client:%s Serial Port:%s]New port added to YAML.",
			r.RemoteAddr, jport.Newname)
		// start newly added port.
		initializereader(jport.Newname)
		return
	}
}

// addPort will add new port configuration first and then
// start port.
func addPort(w http.ResponseWriter, r *http.Request) {
	var jport jsonport
	err := jport.jsondecoder(r)
	if err != nil {
		log.Printf("[Client:%s Serial Port:%s]Error in posted JSON decode:%s",
			r.RemoteAddr, jport.Newname, err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if all.checkElement(jport.Newname) && config.checkElement(jport.Newname) {
		log.Printf("[Client:%s Serial Port:%s]Given port already exist.",
			r.RemoteAddr, jport.Newname)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Port already exist."))
		return
	}

	err = all.addnewport(jport.Newname, jport.Baudrate, 1)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}
	err = config.addElement(jport.Newname, jport.Baudrate, jport.Desc)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}

	err = config.writeYaml(*conf)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Service going into panic. Start again..."))
		log.Panicf("[Client:" + r.RemoteAddr + " Serial Port:" + "]" + jport.Newname + err.Error())
	}
	log.Printf("[Client:%s Serial Port:%s]New port added to YAML.",
		r.RemoteAddr, jport.Newname)
	// start newly added port.
	initializereader(jport.Newname)
	return
}

// deletePort will delete port configuration and cleaup
// struct Config and allports as required and write to yaml configuration file
func deletePort(w http.ResponseWriter, r *http.Request) {
	var pname = r.FormValue("portname")
	msg, status := commonCheck(pname)
	if msg != "" {
		w.WriteHeader(status)
		w.Write([]byte(msg))
		return
	}
	all.ports[pname].clientactive.increment(r.RemoteAddr)

	if all.ports[pname].status == 1 {
		_ = mainReaderClose(pname)
		log.Printf("[Client:%s Serial Port:%s]Main reader go routine closed.",
			r.RemoteAddr, pname)
	}

	_ = all.removeElement(pname)
	log.Printf("[Client:%s Serial Port:%s]Port removed from allports struct.",
		r.RemoteAddr, pname)

	_ = config.removeElement(pname)
	log.Printf("[Client:%s Serial Port:%s]Port removed from config struct.",
		r.RemoteAddr, pname)

	err := config.writeYaml(*conf)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Error writing YAML file."))
		log.Panicf("[Client:" + r.RemoteAddr + " Serial Port:" + "]" + pname + err.Error())
	}
	return
}

// serve static log files
func fileserve(fs http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/x-info")
		fs.ServeHTTP(w, r)
	}
}

// serveHomeHtml handler
func serveHomeHtml(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFiles(absPath + "/ui/home.html")
	if err != nil {
		log.Printf("Home.html serve error:%s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	tmpl.Execute(w, nil)
}

// servePortHtml handler
func servePortHtml(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFiles(absPath + "/ui/port.html")
	if err != nil {
		log.Printf("Port.html serve error:%s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	tmpl.Execute(w, nil)
}

// getConfig will return configuration of yaml file into JSON format
func (c *Config) getConfig(w http.ResponseWriter, r *http.Request) {
	str, err := config.getJSON()
	if err != nil {
		log.Printf("Error in JSON Marshal: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
	} else {
		w.Write(str)
	}
}

// webSocket handler handles any request to access serial port
// over websocket connection
func (p *allports) webSocketHandler(w http.ResponseWriter, r *http.Request) {
	var raddr = r.RemoteAddr
	var pname = r.FormValue("portname")
	log.Printf("[Client:%s Serial Port:%s]Starting session",
		raddr, pname)
	done := make(chan struct{}, 2)
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[Client:%s Serial Port:%s]Websocket upgrade failed: %s",
			raddr, pname, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer func() {
		conn.Close()
		log.Printf("[Client:%s Serial Port:%s]Closed session.",
			raddr, pname)
	}()
	if p.ports[pname].status == 2 {
		log.Printf("[Client:%s Serial Port:%s]Port status is disabled.", raddr, pname)
		conn.WriteMessage(websocket.BinaryMessage, []byte("Please enable port first from UI."))
		return
	}
	// Checking existing number of session and allow or disallow new
	// session.
	if p.ports[pname].clientactive.connection >= 1 {
		msg := "One user session already active with IP:" + p.ports[pname].clientactive.raddr + "."
		conn.WriteMessage(websocket.BinaryMessage, []byte(msg))
		log.Printf("[Client:%s Serial Port:%s]Session not allowed.", raddr, pname)
		return
	} else {
		// p.ports[pname].clientactive.increment(raddr)
		log.Printf("[Client:%s Serial Port:%s]Session allowed. Active session count: %d",
			raddr, pname, p.ports[pname].clientactive.connection)
		p.ports[pname].clientactive.increment(raddr)
	}

	// Checking if port is already open and if not open then return
	// without opening any port read/write.
	if p.ports[pname].port == nil {
		p.ports[pname].clientactive.decrement()
		msg := "Port is not yet opened. Please connect port and try again."
		conn.WriteMessage(websocket.BinaryMessage, []byte(msg))
		log.Printf("[Client:%s Serial Port:%s]Error: %s",
			raddr, pname, msg)
		return
	}

	// goroutine to read from port and write to websocket
	go func() {
		ticker := time.NewTicker(pingPeriod)
		defer func() {
			log.Printf("[Client:%s Serial Port:%s]Go routine write to ws closed.",
				raddr, pname)
			ticker.Stop()
		}()
		for {
			select {
			case v, ok := <-p.ports[pname].comm:
				if !ok {
					log.Printf("[Client:%s Serial Port:%s]Comm channel is closed.",
						raddr, pname)
					done <- struct{}{}
					return
				}
				conn.SetWriteDeadline(time.Now().Add(writeWait))
				err := conn.WriteMessage(websocket.BinaryMessage, []byte(v)[:len([]byte(v))])
				if err != nil {
					log.Printf("[Client:%s Serial Port:%s]Write error %s\n",
						raddr, pname, err)
					done <- struct{}{}
					return
				}
			case <-ticker.C:
				conn.SetWriteDeadline(time.Now().Add(writeWait))
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					log.Printf("[Client:%s Serial Port:%s]Ping write error %s\n",
						raddr, pname, err)
					done <- struct{}{}
					return
				}
			}
		}
	}()

	// goroutine to read from websocket and write to port
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[Client:%s Serial Port:%s]Panic writing to port: %s.",
					raddr, pname, err)
				done <- struct{}{}
			}
			p.ports[pname].clientactive.decrement()
			log.Printf("[Client:%s Serial Port:%s]Go routine read from ws closed.",
				raddr, pname)
		}()
		conn.SetReadDeadline(time.Now().Add(pongWait))
		conn.SetPongHandler(func(string) error {
			conn.SetReadDeadline(time.Now().Add(pongWait))
			return nil
		})
		for {
			_, reader, err := conn.ReadMessage()
			if err != nil {
				log.Printf("[Client:%s Serial Port:%s]Error reading: %s.",
					raddr, pname, err)
				break
			}
			_, err = p.ports[pname].port.Write(reader)
			if err != nil {
				log.Printf("[Client:%s Serial Port:%s]Error writing to port: %s.",
					raddr, pname, err)
				break
			}
		}
		done <- struct{}{}
	}()

	//Wait for either goroutine to exit
	<-done
}

// CreateRouterRegisterPaths will be exported and will create router and register paths.
func createRouterRegisterPaths() *mux.Router {
	path, err := os.Executable()
	if err != nil {
		log.Println("Not able to determine executable path.")
		os.Exit(1)
	}

	// Directory path from where binary started.
	absPath = filepath.Dir(path)

	router := mux.NewRouter().StrictSlash(true)
	registerPaths(router)
	return router
}
