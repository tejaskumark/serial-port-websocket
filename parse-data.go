package main

import (
	"encoding/json"
	"io/ioutil"
	"log"

	"gopkg.in/yaml.v2"
)

// Config type for YAML File marshall/unmarshall
type Config struct {
	Ports []struct {
		Name     string `yaml:"name"`
		Baudrate int    `yaml:"baudrate"`
		Parity   byte   `yaml:"parity"`
		Desc string	`yaml:"desc"`
	} `yaml:"ports"`
	Logs struct {
		Inlogs     string `yaml:"inlogs"`
		Outlogs    string `yaml:"outlogs"`
		Maxsize    int    `yaml:"maxsize"`
		Maxbackups int    `yaml:"maxbackups"`
		Maxage     int    `yaml:"maxage"`
	} `yaml:"logs"`
	ServerConfig []struct {
		Name    string `yaml:"name"`
		Port    int    `yaml:"port"`
		SslCert string `yaml:"sslcert"`
		SslKey  string `yaml:"sslkey"`
	} `yaml:"serverconfig"`
}

// ParseYaml will return err or parse yaml file
func (config *Config) ParseYaml(fileName string) error {
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

func (config *Config) GetJSON() ([]byte, error) {
	b, err := json.Marshal(config)
	if err != nil {
		return []byte(""), err
	}
	return b, err
}
