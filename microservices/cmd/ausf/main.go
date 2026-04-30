package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/alexey/ausf/microservices/internal/api"
	"github.com/alexey/ausf/microservices/internal/config"
	"github.com/alexey/ausf/microservices/internal/controlplane"
	"github.com/alexey/ausf/microservices/internal/metrics"
	"github.com/alexey/ausf/microservices/internal/namf"
	"github.com/alexey/ausf/microservices/internal/service"
)

func main() {
	log.SetFlags(0) // let the JSON lines carry their own timestamp

	appConfig := config.Load()

	reg := metrics.NewRegistry()
	api.SetDefaultRegistry(reg)

	controlPlaneClient := controlplane.NewClientWithBreaker(
		appConfig.ControlPlaneBaseURL,
		appConfig.BreakerFailures,
		time.Duration(appConfig.BreakerTimeoutSeconds)*time.Second,
	)
	namfClient := namf.NewClient(appConfig.NamfBaseURL)
	store, err := service.NewFileAuthContextStore(appConfig.AuthContextStoreFile)
	if err != nil {
		logMain("FATAL", "failed to initialize auth context store", map[string]any{"error": err.Error()})
		os.Exit(1)
	}
	authService := service.NewAuthServiceWithStoreAndTTL(
		controlPlaneClient,
		namfClient,
		store,
		time.Duration(appConfig.AuthContextTTLSeconds)*time.Second,
	)
	authService.SetMetricsRecorder(reg)

	handler := api.NewHandler(authService)

	server := &http.Server{
		Addr:    appConfig.Address(),
		Handler: handler.Routes(),
	}

	logMain("INFO", "AUSF microservice starting", map[string]any{
		"address":       appConfig.Address(),
		"control_plane": appConfig.ControlPlaneBaseURL,
		"namf":          appConfig.NamfBaseURL,
	})
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logMain("FATAL", "server failed", map[string]any{"error": err.Error()})
		os.Exit(1)
	}
}

func logMain(level, msg string, fields map[string]any) {
	fields["time"] = time.Now().UTC().Format(time.RFC3339Nano)
	fields["level"] = level
	fields["msg"] = msg
	data, _ := json.Marshal(fields)
	_, _ = fmt.Fprintln(os.Stderr, string(data))
}
