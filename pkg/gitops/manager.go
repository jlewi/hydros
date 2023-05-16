package gitops

import (
	"fmt"
	"github.com/go-logr/zapr"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"k8s.io/client-go/util/workqueue"
	"sync"
	"time"
)

// Reconciler defines a common interface for reconcilers so that Manager can be used to manage different
// types of reconcilers; i.e. we want to manage both Renderer and Syncer using the Manager interface.
//
// TODO(jeremy): This interface is half baked and we will probably want to refactor it along with the Renderer and
// Syncer structs to implement it.
type Reconciler interface {
	// Name is a unique name for the reconciler
	Name() string
	// Run runs the reconcile loop
	// TODO(jeremy): Should we return a duration which is the time after which to requeue another reconcile event?
	Run() error
}

// Manager manages multiple reconcilers.
// Its job is to ensure each syncer never running more than once
//
// TODO(jeremy): What are the proper semantics for a GitHub reconciler? When does the reconciler get created?
// Should it get created on the first webhook? What should the resync period be? Should we eventually forget
// about a repository if there aren't webhooks in some time period.
type Manager struct {
	// Mapping from the a key to the corresponding syncer
	syncers map[string]Reconciler

	q workqueue.DelayingInterface
	// Wait group is used to detect when all workers have shutdown.
	wg sync.WaitGroup
	mu sync.RWMutex
}

// NewManager starts a new sync manager.
func NewManager(syncers []Reconciler) (*Manager, error) {
	m := &Manager{
		syncers: make(map[string]Reconciler),
		q:       workqueue.NewDelayingQueue(),
	}

	for _, s := range syncers {
		name := s.Name()
		if _, ok := m.syncers[name]; ok {
			return nil, errors.Errorf("Two Reconcilers with name %v; names need to be unique", name)
		}

		m.syncers[name] = s
	}
	return m, nil
}

type DuplicateReconciler struct {
	Name string
}

func (d *DuplicateReconciler) Error() string {
	return fmt.Sprintf("Duplicate reconciler with name %v", d.Name)
}

func IsDuplicateReconciler(err error) bool {
	_, ok := err.(*DuplicateReconciler)
	return ok
}

// AddReconciler adds the reconciler. Returns an DuplicateReconcilerError if a reconciler with the same name already
// If reconciler isn't thread safe then caller should ensure that it isn't called again and let manager take ownership.
func (m *Manager) AddReconciler(r Reconciler) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.syncers[r.Name()]; ok {
		return &DuplicateReconciler{
			Name: r.Name(),
		}
	}

	m.syncers[r.Name()] = r
	return nil
}

func (m *Manager) HasReconciler(name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.syncers[name]
	return ok
}

// Start starts go threads to periodically process the sync objects.
func (m *Manager) Start(numWorkers int, reSyncPeriod time.Duration) error {
	log := zapr.NewLogger(zap.L())
	log.Info("Starting worker threads", "numWorkers", numWorkers)
	for index := 0; index < numWorkers; index++ {
		go m.runWorker(index, reSyncPeriod)
	}

	m.wg.Add(numWorkers)

	for name := range m.syncers {
		// Enqueue an item for each config.
		log.Info("Enqueing config", "name", name)
		m.q.Add(name)
	}

	return nil
}

// Enqueue adds a sync event for the reconciler with the specified name
func (m *Manager) Enqueue(name string) error {
	log := zapr.NewLogger(zap.L())
	log.Info("Enqueing reconcile event", "reconciler", name)
	m.q.Add(name)
	return nil
}

// Shutdown shuts down the syncer's. It will block until all threads have finished.
func (m *Manager) Shutdown() {
	m.q.ShutDown()
	log := zapr.NewLogger(zap.L())
	log.Info("Waiting for workers to shutdown")
	m.wg.Wait()
}

func (m *Manager) runWorker(wid int, reSyncPeriod time.Duration) {
	log := zapr.NewLogger(zap.L()).WithValues("windex", wid)
	for {
		shutdown := func() bool {
			item, shutdown := m.q.Get()
			if shutdown {
				log.Info("worker shutting down")
				return shutdown
			}
			// We need to mark the item as done. Until the item is marked as done further processing is blocked.
			defer m.q.Done(item)
			if _, ok := item.(string); !ok {
				// This is unexpected mark it as done and keep going
				log.Info("Got work queue item which is not a string; %v", item)
				return shutdown
			}
			name := item.(string)
			s, ok := func() (Reconciler, bool) {
				m.mu.RLock()
				defer m.mu.RUnlock()
				r, ok := m.syncers[name]
				return r, ok
			}()

			if !ok {
				log.Info("Error; reconciler with name not found", "name", name)
				return shutdown
			}

			if err := s.Run(); err != nil {
				log.Error(err, "Failed to sync", "name", name)
				return shutdown
			}
			m.q.AddAfter(name, reSyncPeriod)
			return shutdown
		}()

		if shutdown {
			m.wg.Done()
			return
		}
	}
}
