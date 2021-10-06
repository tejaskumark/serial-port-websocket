package main

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"log"
	"sync"

	"gopkg.in/yaml.v2"
)

// port struct as per yaml config
type port struct {
	Name     string `yaml:"name"`
	Baudrate int    `yaml:"baudrate"`
	Parity   byte   `yaml:"parity"`
	Desc     string `yaml:"desc"`
	Status   uint8  `yaml:"status"`
}

// Config type for YAML File marshall/unmarshall
type Config struct {
	mu    sync.Mutex
	Ports []port `yaml:"ports"`
	Logs  struct {
		Inlogs     string `yaml:"inlogs"`
		Maxsize    int    `yaml:"maxsize"`
		Maxbackups int    `yaml:"maxbackups"`
		Maxage     int    `yaml:"maxage"`
	} `yaml:"logs"`
	ServerConfig []struct {
		Name    string `yaml:"name"`
		Enable  int   `yaml:"enable"`
		Port    int    `yaml:"port"`
		SslCert string `yaml:"sslcert"`
		SslKey  string `yaml:"sslkey"`
	} `yaml:"serverconfig"`
}

// writeYaml will write new change config struct to file
func (config *Config) writeYaml(filename string) error {
	config.mu.Lock()
	defer config.mu.Unlock()
	out, err := yaml.Marshal(config)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(filename, out, 0644)
	if err != nil {
		return err
	}
	return nil
}

// parseYaml will return err or parse yaml file
func (config *Config) parseYaml(fileName string) error {
	yamlFile, err := ioutil.ReadFile(fileName)
	if err != nil {
		return err
	}
	err = yaml.Unmarshal(yamlFile, config)
	if err != nil {
		log.Printf("Error : %s", err)
		return err
	}
	return nil
}

// getJSON will convert struct to JSON format and return
// converted byte slice or error
func (config *Config) getJSON() ([]byte, error) {
	config.mu.Lock()
	defer config.mu.Unlock()
	b, err := json.Marshal(config)
	if err != nil {
		return []byte(""), err
	}
	return b, err
}

// removeElement will remove provided port name from ports slice
// if name not found then return false
func (c *Config) removeElement(portname string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for index, value := range c.Ports {
		if value.Name == portname {
			copy(c.Ports[index:], c.Ports[index+1:])
			c.Ports[len(c.Ports)-1] = port{}
			c.Ports = c.Ports[:len(c.Ports)-1]
			return true
		}
	}
	return false
}

// checkElement will check given port into config struct
// and return true if found or false.
func (c *Config) checkElement(portname string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, value := range c.Ports {
		if value.Name == portname {
			return true
		}
	}
	return false
}

// addElement will add new element provided port name, baudrate
// and description
func (c *Config) addElement(portname string, baudrate int, des string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, value := range c.Ports {
		if value.Name == portname {
			return errors.New("port name already exist")
		}
	}
	c.Ports = append(c.Ports, port{Name: portname, Baudrate: baudrate, Desc: des, Status: 1})
	return nil
}

// update element description with given new description
func (c *Config) updateElement(portname string, desc string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for index := range c.Ports {
		if c.Ports[index].Name == portname {
			c.Ports[index].Desc = desc
			return nil
		}
	}
	return errors.New("did not find any element with given port")
}

// getStatus will return port status for a given port
func (c *Config) getStatus(portname string) (uint8, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for index := range c.Ports {
		if c.Ports[index].Name == portname {
			return c.Ports[index].Status, nil
		}
	}
	return 2, errors.New("port not found")
}

// portStatusUpdate will return port status for a given port
func (c *Config) portStatusUpdate(portname string, st uint8) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for index := range c.Ports {
		if c.Ports[index].Name == portname {
			c.Ports[index].Status = st
			return nil
		}
	}
	return errors.New("port not found")
}
