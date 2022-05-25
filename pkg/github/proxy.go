package github

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"strings"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"

	// N.B. We use the Datadog instrumented version of github.com/gorilla/mux
	ddMux "gopkg.in/DataDog/dd-trace-go.v1/contrib/gorilla/mux"
)

// Proxy is a proxy server for GitHub. It proxies http requests using a GitHub app's credentials. This
// makes it easy to fetch documents from private repositories.
//
// N.B jeremy@ tried using the gin framework but couldn't figure out how to properly handle path prefixes.
// I tried using NoRoute and overriding the not found handler but the response code was always 404.
type Proxy struct {
	listener   net.Listener
	log        logr.Logger
	port       string
	transports *TransportManager
}

// NewProxy constructs a new server.
func NewProxy(m *TransportManager, log logr.Logger, port string) (*Proxy, error) {
	if m == nil {
		return nil, fmt.Errorf("TransportManager is required")
	}

	listener, err := net.Listen("tcp", fmt.Sprintf(":%v", port))
	if err != nil {
		return nil, err
	}

	return &Proxy{
		listener:   listener,
		log:        log,
		port:       port,
		transports: m,
	}, nil
}

// Serve starts the server this is blocking.
func (f *Proxy) Serve() error {
	log := f.log

	router := ddMux.NewRouter().StrictSlash(true)
	router.HandleFunc("/healthz", f.HealthCheck)
	router.PathPrefix("/github.com/").HandlerFunc(f.forwardToGithub)
	router.PathPrefix("/raw.githubusercontent.com/").HandlerFunc(f.forwardToGithub)
	router.NotFoundHandler = http.HandlerFunc(f.NotFoundHandler)

	err := http.Serve(f.listener, router)
	if err != nil {
		f.log.Error(err, "Serve returned error")
	}

	log.Info("Echo is running", "address", f.Address())

	return err
}

// Address returns the address the server is listening on.
func (f *Proxy) Address() string {
	return fmt.Sprintf("http://localhost:%v", f.listener.Addr().(*net.TCPAddr).Port)
}

// HealthCheck handles a health check
func (f *Proxy) HealthCheck(w http.ResponseWriter, r *http.Request) {
	_, err := w.Write([]byte("Server is Running!"))
	if err != nil {
		f.log.Error(err, "Failed to write response")
	}
}

type target struct {
	Host string
	Org  string
	Repo string
	Dest string
}

func parseURLPath(path string) (*target, error) {
	orgAndRepo := strings.TrimLeft(path, "/")
	pieces := strings.Split(orgAndRepo, "/")

	if len(pieces) < 3 {
		return nil, fmt.Errorf("Could not get Host and Org from URL path %v", path)
	}

	return &target{
		Host: pieces[0],
		Org:  pieces[1],
		Repo: pieces[2],
		Dest: strings.Join(pieces[3:], "/"),
	}, nil
}

func (t *target) FullPath() string {
	return fmt.Sprintf("%v/%v/%v/%v", t.Host, t.Org, t.Repo, t.Dest)
}

func (f *Proxy) forwardToGithub(w http.ResponseWriter, r *http.Request) {
	log := f.log.WithValues("path", r.URL.Path)

	target, err := parseURLPath(r.URL.Path)
	if err != nil {
		f.writeErrorStatus(w, fmt.Sprintf("Target %v is not in github.com", target.FullPath()), http.StatusBadRequest)
		return
	}
	// For security reasons we only want to forward to github.com
	// Otherwise we are potentially forwarding our credentials to some other server.
	if target.Host != "github.com" && target.Host != "raw.githubusercontent.com" {
		f.writeErrorStatus(w, fmt.Sprintf("Target %v is not in github.com", target.FullPath()), http.StatusBadRequest)
		return
	}

	log = log.WithValues("Org", target.Org, "Repo", target.Repo)
	tr, err := f.transports.Get(target.Org, target.Repo)
	if err != nil {
		log.Error(err, "Failed to create transport")
		f.writeErrorStatus(w, fmt.Sprintf("Failed to create transport; error %v", err), http.StatusInternalServerError)
		return
	}

	// Generate an access token
	token, err := tr.Token(context.Background())
	if err != nil {
		log.Error(err, "Failed to generate access token")
		f.writeErrorStatus(w, fmt.Sprintf("Failed to generate access token; error %v", err), http.StatusInternalServerError)
		return
	}

	// I tried using a header for the x-access-token rather than putting that in the URL but that didn't
	// seem to work.
	fullTarget := fmt.Sprintf("https://x-access-token:%v@%v", token, target.FullPath())
	err = func() error {
		req, err := http.NewRequest(http.MethodGet, fullTarget, nil)
		if err != nil {
			return err
		}
		client := http.DefaultClient
		r, err := client.Do(req)
		if err != nil {
			return err
		}
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			return errors.Wrapf(err, "Failed to read response")
		}
		_, err = w.Write(body)

		if err != nil {
			log.Error(err, "Failed to write response")
			return nil
		}
		return nil
	}()

	if err != nil {
		log.Error(err, "Get failed", "target", target)
		f.writeErrorStatus(w, fmt.Sprintf("Failed to proxy to  %v", target), http.StatusInternalServerError)
		return
	}
}

// NotFoundHandler is a custom not found handler
// A custom not found handler is useful for determining whether a 404 is coming because of an issue with ISTIO
// not hitting the server or the request is hitting the server but the path is wrong.
func (f *Proxy) NotFoundHandler(w http.ResponseWriter, r *http.Request) {
	f.writeErrorStatus(w, fmt.Sprintf("Echo server doesn't serve the requested path; url %v", r.URL), http.StatusNotFound)
}

func (f *Proxy) writeErrorStatus(w http.ResponseWriter, m string, code int) {
	log := f.log
	// We need to call writeHeader before WriteBody otherwise WriteBody will set the code to 200 and it won't get
	// reset.
	w.WriteHeader(code)
	_, _ = w.Write([]byte(m))

	// Log the errors
	log.Info("writeErrorStatus called", "message", m, "code", code)
}
