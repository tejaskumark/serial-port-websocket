package main

import (
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
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
	str, err := config.GetJSON()
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

	// Checking existing number of session and allow or disallow new
	// session.
	if p.ports[pname].clientactive.connection >= 1 {
		msg := "One user session already active with IP: " + raddr + "."
		conn.WriteMessage(websocket.BinaryMessage, []byte(msg))
		log.Printf("[Client:%s Serial Port:%s]Session not allowed. %d",
			raddr, pname, p.ports[pname].clientactive.connection)
		return
	} else {
		p.ports[pname].clientactive.increment()
		log.Printf("[Client:%s Serial Port:%s]Session allowed. Active session count: %d",
			raddr, pname, p.ports[pname].clientactive.connection)
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
