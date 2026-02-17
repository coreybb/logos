package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	rh "github.com/coreybb/logos/route-handlers"
	"github.com/coreybb/logos/webutil"
)

const (
	apiBasePath              = "/api"
	usersBasePath            = "/users"
	readingsBasePath         = "/readings"
	editionsBasePath         = "/editions"
	deliveriesBasePath       = "/deliveries"
	sourcesBasePath          = "/sources"
	destinationsBasePath     = "/destinations"
	editionTemplatesBasePath = "/edition-templates"
)

const (
	readingsSubPath      = "/readings"
	statusSubPath        = "/status"
	subscriptionsSubPath = "/subscriptions" // For user subscriptions to sources
)

const (
	paramID = "id" // General parameter name for resource IDs
)

func SetupRoutes(
	userHandler *rh.UserHandler,
	editionHandler *rh.EditionHandler,
	readingHandler *rh.ReadingHandler,
	deliveryHandler *rh.DeliveryHandler,
	sourceHandler *rh.SourceHandler,
	destinationHandler *rh.DestinationHandler,
	editionTemplateHandler *rh.EditionTemplateHandler,
	userReadingSourceHandler *rh.UserReadingSourceHandler,
) http.Handler {
	r := chi.NewRouter()

	// Middleware stack
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)                                                 // Log every request
	r.Use(middleware.Recoverer)                                              // Recover from panics
	r.Use(middleware.Timeout(60 * time.Second))                              // Set a timeout context for requests
	r.Use(SetHeader(webutil.HeaderContentType, webutil.ContentTypeJSONUTF8)) // Default Content-Type

	// API versioning or grouping
	r.Route(apiBasePath, func(r chi.Router) {
		configureUserRoutes(r, userHandler, readingHandler)
		configureEditionRoutes(r, editionHandler)
		configureReadingRoutes(r, readingHandler)
		configureDeliveryRoutes(r, deliveryHandler)
		configureSourceRoutes(r, sourceHandler)
		configureDestinationRoutes(r, destinationHandler)
		configureEditionTemplateRoutes(r, editionTemplateHandler)
		configureUserSubscriptionRoutes(r, userReadingSourceHandler)
	})

	// Health check endpoint
	r.Get("/healthz", handleHealthCheck)

	return r
}

// Helper for constructing paths with a parameter
func pathWithParam(basePath string, paramName string) string {
	if basePath == "" {
		return "/{" + paramName + "}"
	}
	return basePath + "/{" + paramName + "}"
}

// --- User Routes ---
func configureUserRoutes(r chi.Router, userHandler *rh.UserHandler, readingHandler *rh.ReadingHandler) {
	userSpecificPath := pathWithParam("", paramID) // e.g., "/{id}"

	r.Route(usersBasePath, func(r chi.Router) {
		r.Get("/", webutil.MakeHandler(userHandler.HandleGetUsers))
		r.Post("/", webutil.MakeHandler(userHandler.HandleCreateUser))
		r.Route(userSpecificPath, func(r chi.Router) {
			r.Get("/", webutil.MakeHandler(userHandler.HandleGetUser))
			// Nested: Get readings for a specific user
			r.Get(readingsSubPath, webutil.MakeHandler(readingHandler.HandleGetUserReadings)) // GET /users/{id}/readings
		})
	})
}

// --- Reading Routes ---
func configureReadingRoutes(r chi.Router, handler *rh.ReadingHandler) {
	specificReadingPath := pathWithParam("", paramID) // e.g., "/{id}"

	r.Route(readingsBasePath, func(r chi.Router) {
		r.Get("/", webutil.MakeHandler(handler.HandleGetReadings))
		r.Post("/", webutil.MakeHandler(handler.HandleCreateReading))
		r.Get(specificReadingPath, webutil.MakeHandler(handler.HandleGetReading))
	})
}

// --- Edition Routes ---
func configureEditionRoutes(r chi.Router, handler *rh.EditionHandler) {
	specificEditionPath := pathWithParam("", paramID) // e.g., "/{id}"

	r.Route(editionsBasePath, func(r chi.Router) {
		r.Get("/", webutil.MakeHandler(handler.HandleGetEditions))    // Query param for user_id
		r.Post("/", webutil.MakeHandler(handler.HandleCreateEdition)) // UserID in body
		r.Route(specificEditionPath, func(r chi.Router) {
			r.Get("/", webutil.MakeHandler(handler.HandleGetEdition))
			// Nested: Readings for an edition
			r.Get(readingsSubPath, webutil.MakeHandler(handler.HandleGetEditionReadings))   // GET /editions/{id}/readings
			r.Post(readingsSubPath, webutil.MakeHandler(handler.HandleAddReadingToEdition)) // POST /editions/{id}/readings
			// Generate document for an edition
			r.Post("/generate", webutil.MakeHandler(handler.HandleGenerateEditionDocument)) // POST /editions/{id}/generate
		})
	})
}

// --- Delivery Routes ---
func configureDeliveryRoutes(r chi.Router, handler *rh.DeliveryHandler) {
	specificDeliveryPath := pathWithParam("", paramID) // e.g., "/{id}"

	r.Route(deliveriesBasePath, func(r chi.Router) {
		r.Post("/", webutil.MakeHandler(handler.HandleCreateDelivery))
		r.Route(specificDeliveryPath, func(r chi.Router) {
			r.Get("/", webutil.MakeHandler(handler.HandleGetDelivery))
			// Nested: Update status for a specific delivery
			r.Patch(statusSubPath, webutil.MakeHandler(handler.HandleUpdateDeliveryStatus)) // PATCH /deliveries/{id}/status
		})
	})
}

// --- Reading Source Routes ---
func configureSourceRoutes(r chi.Router, handler *rh.SourceHandler) {
	specificSourcePath := pathWithParam("", paramID) // e.g., "/{id}"

	r.Route(sourcesBasePath, func(r chi.Router) {
		r.Get("/", webutil.MakeHandler(handler.HandleGetSources))
		r.Post("/", webutil.MakeHandler(handler.HandleCreateSource))
		r.Get(specificSourcePath, webutil.MakeHandler(handler.HandleGetSourceByID))
	})
}

// --- Delivery Destination Routes ---
func configureDestinationRoutes(r chi.Router, handler *rh.DestinationHandler) {
	// specificDestinationPath := pathWithParam("", paramID) // e.g., "{id}"

	r.Route(destinationsBasePath, func(r chi.Router) {
		r.Get("/", webutil.MakeHandler(handler.HandleGetDestinations))    // Query param for user_id
		r.Post("/", webutil.MakeHandler(handler.HandleCreateDestination)) // UserID in body
		// Example for getting specific destination if needed later:
		// r.Get(specificDestinationPath, webutil.MakeHandler(handler.HandleGetDestinationByID))
	})
}

// --- Edition Template Routes ---
func configureEditionTemplateRoutes(r chi.Router, handler *rh.EditionTemplateHandler) {
	// Route for creating a new edition template. UserID will be in the request body.
	r.Post(editionTemplatesBasePath, webutil.MakeHandler(handler.HandleCreateEditionTemplate))

	// Routes for operations on edition templates scoped by user.
	// Path: /users/{user_id_for_templates}/edition-templates
	// Path: /users/{user_id_for_templates}/edition-templates/{id} (template's id)
	userScopedTemplatesPath := usersBasePath + pathWithParam("", "user_id_for_templates") + editionTemplatesBasePath

	r.Route(userScopedTemplatesPath, func(r chi.Router) {
		// GET /users/{user_id_for_templates}/edition-templates
		r.Get("/", webutil.MakeHandler(handler.HandleGetEditionTemplatesByUserID))

		// Operations on a specific template for that user
		// paramID here will be the template's ID.
		// The handler needs to get "user_id_for_templates" and "id" from path.
		r.Route(pathWithParam("", paramID), func(r chi.Router) {
			r.Get("/", webutil.MakeHandler(handler.HandleGetEditionTemplateByID))
			r.Put("/", webutil.MakeHandler(handler.HandleUpdateEditionTemplate))
			r.Delete("/", webutil.MakeHandler(handler.HandleDeleteEditionTemplate))
		})
	})
}

// --- User Subscription Routes (to Reading Sources) ---
func configureUserSubscriptionRoutes(r chi.Router, handler *rh.UserReadingSourceHandler) {
	// Base path for a user's subscriptions: /users/{userID}/subscriptions
	// Specific subscription: /users/{userID}/subscriptions/{sourceID}
	// The handlers expect "userID" and "sourceID" from the path parameters.
	userSubscriptionsPath := usersBasePath + pathWithParam("", "userID") + subscriptionsSubPath

	r.Route(userSubscriptionsPath, func(r chi.Router) {
		// GET /users/{userID}/subscriptions -> Get all sources user is subscribed to
		r.Get("/", webutil.MakeHandler(handler.HandleGetUserSubscribedSources))

		// Operations on a specific subscription
		// paramID here will be the source's ID.
		r.Route(pathWithParam("", "sourceID"), func(r chi.Router) {
			r.Post("/", webutil.MakeHandler(handler.HandleSubscribeUserToSource))       // Subscribe
			r.Delete("/", webutil.MakeHandler(handler.HandleUnsubscribeUserFromSource)) // Unsubscribe
		})
	})
}

// --- Utility Functions ---

// handleHealthCheck responds to a health check request.
func handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set(webutil.HeaderContentType, webutil.ContentTypeTextPlainUTF8)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

// SetHeader is a middleware to set a response header.
func SetHeader(key, value string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set(key, value)
			next.ServeHTTP(w, r)
		})
	}
}
