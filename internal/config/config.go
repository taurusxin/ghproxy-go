// Package config loads the service's environment settings.
package config

import (
	"net"
	"os"
)

type Config struct {
	Host string
	Port string
}

func FromEnv() Config {
	host := os.Getenv("HOST")
	if host == "" {
		host = "0.0.0.0"
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8972"
	}
	return Config{Host: host, Port: port}
}

func (c Config) Address() string {
	return net.JoinHostPort(c.Host, c.Port)
}
