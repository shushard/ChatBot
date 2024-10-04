package main

import (
	"context"
	"github.com/shushard/ChatBot/internal"
	"os"

	"github.com/rs/zerolog"
	"github.com/shushard/ChatBot/internal/config"
)

func main() {
	// Initialize logger
	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()

	// Load or define your configuration
	conf := config.Config{
		SavePath: "videos", // Adjust the save path as needed
		Headless: false,    // Set to true if you want to run in headless mode
		SiteConfigs: []config.SiteConfig{
			{
				SiteURL: "https://discord.com/",
				// Add other site-specific configurations if needed
			},
		},
	}

	// Create the Service
	service, err := internal.New(conf, &logger)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to create service")
		os.Exit(1)
	}

	// Run the Service
	ctx := context.Background()
	if err := service.Run(ctx); err != nil {
		logger.Error().Err(err).Msg("Service run failed")
		os.Exit(1)
	}

	// Optionally, shutdown the service when done
	if err := service.Shutdown(ctx); err != nil {
		logger.Error().Err(err).Msg("Service shutdown failed")
	}
}
