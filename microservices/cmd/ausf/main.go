package main

import (
    "log"
    "net/http"

    "github.com/alexey/ausf/microservices/internal/api"
    "github.com/alexey/ausf/microservices/internal/config"
    "github.com/alexey/ausf/microservices/internal/controlplane"
    "github.com/alexey/ausf/microservices/internal/service"
)

func main() {
    appConfig := config.Load()
    controlPlaneClient := controlplane.NewClient(appConfig.ControlPlaneBaseURL)
    authService := service.NewAuthService(controlPlaneClient)
    handler := api.NewHandler(authService)

    server := &http.Server{
        Addr:    appConfig.Address(),
        Handler: handler.Routes(),
    }

    log.Printf("AUSF microservice listening on %s and using control-plane %s", appConfig.Address(), appConfig.ControlPlaneBaseURL)
    if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
        log.Fatalf("server failed: %v", err)
    }
}
