package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/coreybb/logos/api"
	"github.com/coreybb/logos/datastore"
	"github.com/coreybb/logos/delivery"
	"github.com/coreybb/logos/ebook"
	"github.com/coreybb/logos/processing"
	rh "github.com/coreybb/logos/route-handlers"
	"github.com/coreybb/logos/scheduler"
	"github.com/coreybb/logos/webhooks"
	"github.com/go-chi/chi/v5"
	_ "github.com/lib/pq"
)

const (
	defaultPort         = "8080"
	defaultDatabaseURL  = "user=postgres password=password dbname=logos host=localhost port=5432 sslmode=disable"
	defaultSendGridFrom = "deliver@lakonic.dev"
	defaultSendGridName = "Logos"
	dbPingTimeout       = 5 * time.Second
	shutdownTimeout     = 15 * time.Second
	dbMaxOpenConns      = 25
	dbMaxIdleConns      = 25
	dbConnMaxLifetime   = 5 * time.Minute
)

type config struct {
	port              string
	databaseURL       string
	sendGridAPIKey    string
	sendGridFromEmail string
	sendGridFromName  string
}

func main() {
	cfg := loadConfig()

	db, err := setupDatabase(cfg.databaseURL)
	if err != nil {
		log.Fatalf("Database setup failed: %v", err)
	}
	defer db.Close()

	userRepo := datastore.NewUserRepository(db)
	editionRepo := datastore.NewEditionRepository(db)
	readingRepo := datastore.NewReadingRepository(db)
	deliveryRepo := datastore.NewDeliveryRepository(db)
	sourceRepo := datastore.NewSourceRepository(db)
	destinationRepo := datastore.NewDestinationRepository(db)
	editionTemplateRepo := datastore.NewEditionTemplateRepository(db)
	userReadingSourceRepo := datastore.NewUserReadingSourceRepository(db)
	editionTemplateSourceRepo := datastore.NewEditionTemplateSourceRepository(db)
	allowedSenderRepo := datastore.NewAllowedSenderRepository(db)
	deliveryAttemptRepo := datastore.NewDeliveryAttemptRepository(db)

	// Initialize ebook generator
	editionGenerator := ebook.NewEditionGenerator()

	// Initialize edition processor
	editionProcessor := processing.NewEditionProcessor(
		editionRepo,
		readingRepo,
		deliveryRepo,
		editionTemplateRepo,
		editionGenerator,
	)

	// Initialize delivery system
	emailProvider := delivery.NewEmailDeliveryProvider(cfg.sendGridAPIKey, cfg.sendGridFromEmail, cfg.sendGridFromName)
	deliveryService := delivery.NewDeliveryService(deliveryRepo, destinationRepo, deliveryAttemptRepo, emailProvider)

	userHandler := rh.NewUserHandler(userRepo)
	editionHandler := rh.NewEditionHandler(editionRepo, editionProcessor, deliveryService)
	readingHandler := rh.NewReadingHandler(readingRepo)
	deliveryHandler := rh.NewDeliveryHandler(deliveryRepo)
	sourceHandler := rh.NewSourceHandler(sourceRepo)
	destinationHandler := rh.NewDestinationHandler(destinationRepo)
	editionTemplateHandler := rh.NewEditionTemplateHandler(editionTemplateRepo)
	userReadingSourceHandler := rh.NewUserReadingSourceHandler(userReadingSourceRepo)
	editionTemplateSourceHandler := rh.NewEditionTemplateSourceHandler(editionTemplateSourceRepo)
	allowedSenderHandler := rh.NewAllowedSenderHandler(allowedSenderRepo)

	inboundEmailHandler := webhooks.NewInboundEmailHandler(readingRepo, sourceRepo, allowedSenderRepo)

	apiRouter := api.SetupRoutes(
		userHandler,
		editionHandler,
		readingHandler,
		deliveryHandler,
		sourceHandler,
		destinationHandler,
		editionTemplateHandler,
		userReadingSourceHandler,
		editionTemplateSourceHandler,
		allowedSenderHandler,
	)

	// Initialize scheduler
	editionScheduler := scheduler.New(
		editionTemplateRepo,
		editionTemplateSourceRepo,
		editionRepo,
		readingRepo,
		destinationRepo,
		editionProcessor,
		deliveryService,
	)

	mainRouter := chi.NewRouter()
	mainRouter.Mount("/", apiRouter)

	mainRouter.Post("/webhooks/inbound-email", inboundEmailHandler.HandleInbound)
	mainRouter.Post("/scheduler/tick", editionScheduler.HandleTick)

	startServer(cfg.port, mainRouter)
}

func loadConfig() config {
	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}

	dbURL := os.Getenv("DB_CONNECTION_STRING")
	if dbURL == "" {
		dbURL = defaultDatabaseURL
		log.Println("WARNING: DB_CONNECTION_STRING not set, using default local connection string.")
	}

	sendGridAPIKey := os.Getenv("SENDGRID_API_KEY")
	if sendGridAPIKey == "" {
		log.Println("WARNING: SENDGRID_API_KEY not set. Email delivery will fail at runtime.")
	}

	sendGridFrom := os.Getenv("SENDGRID_FROM_EMAIL")
	if sendGridFrom == "" {
		sendGridFrom = defaultSendGridFrom
	}

	sendGridName := os.Getenv("SENDGRID_FROM_NAME")
	if sendGridName == "" {
		sendGridName = defaultSendGridName
	}

	return config{
		port:              port,
		databaseURL:       dbURL,
		sendGridAPIKey:    sendGridAPIKey,
		sendGridFromEmail: sendGridFrom,
		sendGridFromName:  sendGridName,
	}
}

func setupDatabase(connStr string) (*sql.DB, error) {
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection: %w", err)
	}

	db.SetMaxOpenConns(dbMaxOpenConns)
	db.SetMaxIdleConns(dbMaxIdleConns)
	db.SetConnMaxLifetime(dbConnMaxLifetime)

	ctx, cancel := context.WithTimeout(context.Background(), dbPingTimeout)
	defer cancel()

	if err = db.PingContext(ctx); err != nil {
		db.Close() // Close unusable connection pool
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	log.Println("Database connection successful")
	return db, nil
}

func startServer(port string, router http.Handler) {
	server := &http.Server{
		Addr:    ":" + port,
		Handler: router,
	}

	shutdownSignal := make(chan os.Signal, 1)
	signal.Notify(shutdownSignal, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("Server starting on port %s", port)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Server error: %v", err)
		}
	}()

	<-shutdownSignal // Block until signal received
	log.Println("Shutdown signal received, initiating graceful shutdown...")

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancelShutdown()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Graceful shutdown failed: %v", err)
	}

	log.Println("Server gracefully stopped")
}
