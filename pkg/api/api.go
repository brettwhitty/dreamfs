package api

import (
	"context"
	"embed"
	"io/fs"
	"net/http"
	"runtime"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"gnomatix/dreamfs/v2/pkg/storage"
	"gnomatix/dreamfs/v2/pkg/utils"
)

//go:embed static
var staticFiles embed.FS

// IndexerState holds live status about the currently running indexer job.
// It is updated safely under a mutex by the fileprocessor and read by the API.
type IndexerState struct {
	mu          sync.RWMutex
	Running     bool   `json:"running"`
	CurrentDir  string `json:"current_dir"`
	Processed   int64  `json:"processed"`
	Total       int64  `json:"total"`
	StartedAt   string `json:"started_at"`
	volumeRoot  string
	cancelFn    context.CancelFunc
}

func NewIndexerState() *IndexerState {
	return &IndexerState{}
}

func (s *IndexerState) SetRunning(root string, cancel context.CancelFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Running = true
	s.volumeRoot = root
	s.CurrentDir = root
	s.Processed = 0
	s.Total = 0
	s.StartedAt = time.Now().Format(time.RFC3339)
	s.cancelFn = cancel
}

func (s *IndexerState) SetIdle() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Running = false
	s.cancelFn = nil
}

func (s *IndexerState) Update(dir string, processed, total int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.CurrentDir = dir
	s.Processed = processed
	s.Total = total
}

func (s *IndexerState) Snapshot() IndexerState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return IndexerState{
		Running:    s.Running,
		CurrentDir: s.CurrentDir,
		Processed:  s.Processed,
		Total:      s.Total,
		StartedAt:  s.StartedAt,
	}
}

func (s *IndexerState) Cancel() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancelFn != nil {
		s.cancelFn()
		return true
	}
	return false
}

// Server holds dependencies for the Gin API server.
type Server struct {
	router  *gin.Engine
	ps      *storage.PersistentStore
	Indexer *IndexerState
	startedAt time.Time
}

// NewServer creates and configures the Gin server.
func NewServer(ps *storage.PersistentStore, indexer *IndexerState) *Server {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	s := &Server{
		router:    r,
		ps:        ps,
		Indexer:   indexer,
		startedAt: time.Now(),
	}
	s.registerRoutes()
	return s
}

// Run starts the Gin HTTP server on the given address (e.g. ":8080").
// It blocks until the context is cancelled.
func (s *Server) Run(ctx context.Context, addr string) error {
	srv := &http.Server{
		Addr:    addr,
		Handler: s.router,
	}
	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutCtx)
	}()
	return srv.ListenAndServe()
}

func (s *Server) registerRoutes() {
	// Serve embedded static web console.
	staticFS, _ := fs.Sub(staticFiles, "static")
	s.router.StaticFS("/ui", http.FS(staticFS))
	// Redirect root to the console.
	s.router.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusFound, "/ui/")
	})

	v1 := s.router.Group("/api/v1")
	{
		v1.GET("/status", s.handleStatus)
		v1.GET("/volumes", s.handleVolumes)
		v1.GET("/volumes/:id/files", s.handleVolumeFiles)
		v1.GET("/indexer/progress", s.handleIndexerProgress)
		v1.POST("/indexer/stop", s.handleIndexerStop)
		v1.GET("/config", s.handleConfig)
	}
}

// ------------------------------------------------------------------
// Handlers
// ------------------------------------------------------------------

func (s *Server) handleStatus(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"host_id":    utils.HostID,
		"uptime_sec": time.Since(s.startedAt).Seconds(),
		"go_version": runtime.Version(),
		"os":         runtime.GOOS,
		"arch":       runtime.GOARCH,
		"num_cpu":    runtime.NumCPU(),
	})
}

func (s *Server) handleVolumes(c *gin.Context) {
	// Return unique volume IDs seen in the metadata store.
	all, err := s.ps.GetAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	seen := map[string]struct{}{}
	for _, m := range all {
		if vid, ok := m.Extra["volume_id"].(string); ok && vid != "" {
			seen[vid] = struct{}{}
		}
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	c.JSON(http.StatusOK, gin.H{"volumes": ids, "count": len(ids)})
}

func (s *Server) handleVolumeFiles(c *gin.Context) {
	volumeID := c.Param("id")
	all, err := s.ps.GetAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var filtered []interface{}
	for _, m := range all {
		if vid, ok := m.Extra["volume_id"].(string); ok && vid == volumeID {
			filtered = append(filtered, m)
		}
	}
	if filtered == nil {
		filtered = []interface{}{}
	}
	c.JSON(http.StatusOK, gin.H{"volume_id": volumeID, "files": filtered, "count": len(filtered)})
}

func (s *Server) handleIndexerProgress(c *gin.Context) {
	snap := s.Indexer.Snapshot()
	c.JSON(http.StatusOK, snap)
}

func (s *Server) handleIndexerStop(c *gin.Context) {
	if s.Indexer.Cancel() {
		c.JSON(http.StatusOK, gin.H{"status": "cancelled"})
	} else {
		c.JSON(http.StatusConflict, gin.H{"status": "not_running"})
	}
}

func (s *Server) handleConfig(c *gin.Context) {
	// Return a safe subset of viper config (no secrets).
	c.JSON(http.StatusOK, gin.H{
		"host_id": utils.HostID,
		"note":    "Full viper config available in later release",
	})
}
