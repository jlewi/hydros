package ghapp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/gorilla/mux"
	"github.com/jlewi/hydros/pkg/util"
	"github.com/palantir/go-githubapp/githubapp"

	// TODO(jeremy): We should move relevant code in jlewi/p22h to jlewi/monogo
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/jlewi/p22h/backend/api"
	"github.com/jlewi/p22h/backend/pkg/debug"
	"go.uber.org/zap"
)

const (
	healthPath = "/healthz"
	UserAgent  = "hydros/0.0.1"
)

// Server is the server that wraps hydros in order to handle webhooks
//
// TODO(jeremy): We might want to add a queue similar to what we did in flock to handle multiple syncs
type Server struct {
	log  logr.Logger
	srv  *http.Server
	port int

	router *mux.Router

	config  githubapp.Config
	handler *HydrosHandler
	// Handler for GitHub webhooks
	gitWebhook http.Handler

	baseHREF string
}

// NewServer creates a new server that relies on IAP as an authentication proxy.
func NewServer(baseHREF string, port int, config githubapp.Config, handler *HydrosHandler) (*Server, error) {
	// Strip trailing slash from baseHREF so we can just it to the url path
	if strings.HasSuffix(baseHREF, "/") {
		baseHREF = baseHREF[:len(baseHREF)-1]
	}
	s := &Server{
		log:      zapr.NewLogger(zap.L()),
		port:     port,
		config:   config,
		handler:  handler,
		baseHREF: baseHREF,
	}

	if err := s.setupHandler(); err != nil {
		return nil, err
	}
	if err := s.addRoutes(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Server) setupHandler() error {
	if s.config.App.WebhookSecret == "" {
		return fmt.Errorf("WebhookSecret must be set")
	}
	s.gitWebhook = githubapp.NewEventDispatcher([]githubapp.EventHandler{s.handler}, s.config.App.WebhookSecret, githubapp.WithErrorCallback(LogErrorCallback))
	return nil
}

// StartAndBlock starts the server and blocks.
func (s *Server) StartAndBlock() {
	log := s.log
	log.Info("Binding all network interfaces", "port", s.port)
	s.srv = &http.Server{Addr: fmt.Sprintf(":%v", s.port), Handler: s.router}

	s.trapInterrupt()
	err := s.srv.ListenAndServe()

	if err != nil {
		if err == http.ErrServerClosed {
			log.Info("Hydros has been shutdown")
		} else {
			log.Error(err, "Server aborted with error")
		}
	}
}

func (s *Server) addRoutes() error {
	log := zapr.NewLogger(zap.L())
	router := mux.NewRouter().StrictSlash(true)
	s.router = router

	hPath := s.baseHREF + healthPath
	log.Info("Registering health check", "path", hPath)
	router.HandleFunc(hPath, s.healthCheck)

	githubWebhookPath := s.baseHREF + githubapp.DefaultWebhookRoute
	log.Info("Adding routes for GitHub webhooks", "path", githubWebhookPath)
	router.Handle(githubWebhookPath, s.gitWebhook)
	router.NotFoundHandler = http.HandlerFunc(s.notFoundHandler)

	return nil
}

// trapInterrupt waits for a shutdown signal and shutsdown the server
func (s *Server) trapInterrupt() {
	sigs := make(chan os.Signal, 10)
	// SIGSTOP and SIGTERM can't be caught; however SIGINT works as expected when using ctl-z
	// to interrupt the process
	signal.Notify(sigs, syscall.SIGINT)

	go func() {
		msg := <-sigs
		s.log.Info("Received shutdown signal", "sig", msg)
		if err := s.srv.Shutdown(context.Background()); err != nil {
			s.log.Error(err, "Error shutting down server.")
		}
	}()
}

func (s *Server) writeStatus(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)

	resp := api.RequestStatus{
		Kind:    "RequestStatus",
		Message: message,
		Code:    code,
	}

	enc := json.NewEncoder(w)
	if err := enc.Encode(resp); err != nil {
		s.log.Error(err, "Failed to marshal RequestStatus", "RequestStatus", resp, "code", code)
	}

	if code != http.StatusOK {
		caller := debug.ThisCaller()
		s.log.Info("HTTP error", "RequestStatus", resp, "code", code, "caller", caller)
	}
}

func (s *Server) healthCheck(w http.ResponseWriter, r *http.Request) {
	s.log.V(util.Debug).Info("Call to /healthz")
	s.writeStatus(w, "Hydros server is running", http.StatusOK)
}

func (s *Server) notFoundHandler(w http.ResponseWriter, r *http.Request) {
	s.writeStatus(w, fmt.Sprintf("Hydros server doesn't handle the path; url: %v", r.URL), http.StatusNotFound)
}

// LogErrorCallback handles errors by logging them
func LogErrorCallback(w http.ResponseWriter, r *http.Request, err error) {
	log := zapr.NewLogger(zap.L())
	log = log.WithValues("githubHookID", r.Header.Get("X-GitHub-Hook-ID"))
	log = log.WithValues("eventType", r.Header.Get("X-GitHub-Event"))
	log = log.WithValues("deliverID", r.Header.Get("X-GitHub-Delivery"))
	log.Error(err, "Failed to handle GitHub webhook")
}
