package main

import (
	"log"
	"net/http"
	"time"

	"github.com/alexey/ausf/microservices/internal/api"
	"github.com/alexey/ausf/microservices/internal/config"
	"github.com/alexey/ausf/microservices/internal/controlplane"
	"github.com/alexey/ausf/microservices/internal/namf"
	"github.com/alexey/ausf/microservices/internal/service"
)

func main() {
	appConfig := config.Load()
	controlPlaneClient := controlplane.NewClient(appConfig.ControlPlaneBaseURL)
	namfClient := namf.NewClient(appConfig.NamfBaseURL)
	store, err := service.NewFileAuthContextStore(appConfig.AuthContextStoreFile)
	if err != nil {
		log.Fatalf("failed to initialize auth context store: %v", err)
	}
	authService := service.NewAuthServiceWithStoreAndTTL(
		controlPlaneClient,
		namfClient,
		store,
		time.Duration(appConfig.AuthContextTTLSeconds)*time.Second,
	)
	handler := api.NewHandler(authService)

	server := &http.Server{
		Addr:    appConfig.Address(),
		Handler: handler.Routes(),
	}

	log.Printf(
		"AUSF microservice listening on %s and using control-plane %s (namf=%s)",
		appConfig.Address(),
		appConfig.ControlPlaneBaseURL,
		appConfig.NamfBaseURL,
	)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server failed: %v", err)
	}
}
