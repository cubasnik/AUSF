package config

import (
    "fmt"
    "os"
)

type Config struct {
    Host                string
    Port                string
    ControlPlaneBaseURL string
}

func Load() Config {
    host := os.Getenv("AUSF_HOST")
    if host == "" {
        host = "0.0.0.0"
    }

    port := os.Getenv("AUSF_PORT")
    if port == "" {
        port = "8080"
    }

    controlPlaneBaseURL := os.Getenv("CONTROL_PLANE_BASE_URL")
    if controlPlaneBaseURL == "" {
        controlPlaneBaseURL = "http://127.0.0.1:8081"
    }

    return Config{Host: host, Port: port, ControlPlaneBaseURL: controlPlaneBaseURL}
}

func (config Config) Address() string {
    return fmt.Sprintf("%s:%s", config.Host, config.Port)
}
