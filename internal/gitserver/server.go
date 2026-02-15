package gitserver

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/mycelium-dev/mycelium/internal/config"
)

// Server wraps the Soft Serve git server with Mycelium-specific functionality
type Server struct {
	config    *config.Config
	dataDir   string
	ctx       context.Context
	cancel    context.CancelFunc
	logger    *slog.Logger
	
	// TODO: Will be replaced with actual Soft Serve server once we add the dependency
	// softServeServer *server.Server
}

// NewServer creates a new git server instance
func NewServer(cfg *config.Config) (*Server, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	// Ensure data directory exists
	dataDir := cfg.Server.DataDir
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory %s: %w", dataDir, err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Server{
		config:  cfg,
		dataDir: dataDir,
		ctx:     ctx,
		cancel:  cancel,
		logger:  slog.Default(),
	}, nil
}

// Start starts the git server
func (s *Server) Start() error {
	s.logger.Info("Starting Mycelium git server",
		"host", s.config.Server.Host,
		"port", s.config.Server.Port,
		"dataDir", s.dataDir)

	// TODO: Implement actual Soft Serve integration
	// For now, this is a placeholder that demonstrates the interface
	
	// Setup would include:
	// 1. Configure Soft Serve with our settings
	// 2. Set up SSH keys
	// 3. Initialize database
	// 4. Start the server
	
	s.logger.Info("Git server started successfully",
		"ssh_url", fmt.Sprintf("ssh://%s:%d", s.config.Server.Host, s.config.Server.Port))
	
	// This will be replaced with: return s.softServeServer.Start()
	return nil
}

// Stop gracefully shuts down the git server
func (s *Server) Stop() error {
	s.logger.Info("Stopping Mycelium git server")
	
	// Cancel context to signal shutdown
	s.cancel()
	
	// TODO: Implement actual Soft Serve shutdown
	// This will be replaced with: return s.softServeServer.Shutdown(ctx)
	
	s.logger.Info("Git server stopped")
	return nil
}

// CreateRepo creates a new repository programmatically
// This will be used by the background worker to create repos for extracted skills
func (s *Server) CreateRepo(name, description string) error {
	if name == "" {
		return fmt.Errorf("repository name cannot be empty")
	}

	s.logger.Info("Creating repository", "name", name, "description", description)
	
	// TODO: Implement repository creation via Soft Serve API
	// This would typically involve:
	// 1. Validate repository name
	// 2. Create bare git repository
	// 3. Set up repository metadata
	// 4. Configure access permissions
	
	// For now, create a placeholder directory structure
	repoPath := filepath.Join(s.dataDir, "repos", name+".git")
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		return fmt.Errorf("failed to create repository directory: %w", err)
	}
	
	s.logger.Info("Repository created successfully", "name", name, "path", repoPath)
	return nil
}

// ListRepos returns a list of all repositories
func (s *Server) ListRepos() ([]string, error) {
	s.logger.Debug("Listing repositories")
	
	// TODO: Implement actual repository listing via Soft Serve API
	
	reposDir := filepath.Join(s.dataDir, "repos")
	if _, err := os.Stat(reposDir); os.IsNotExist(err) {
		return []string{}, nil
	}
	
	entries, err := os.ReadDir(reposDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read repositories directory: %w", err)
	}
	
	var repos []string
	for _, entry := range entries {
		if entry.IsDir() && entry.Name() != ".git" {
			// Remove .git suffix if present
			name := entry.Name()
			if len(name) > 4 && name[len(name)-4:] == ".git" {
				name = name[:len(name)-4]
			}
			repos = append(repos, name)
		}
	}
	
	s.logger.Debug("Found repositories", "count", len(repos), "repos", repos)
	return repos, nil
}

// DeleteRepo removes a repository
func (s *Server) DeleteRepo(name string) error {
	if name == "" {
		return fmt.Errorf("repository name cannot be empty")
	}
	
	s.logger.Info("Deleting repository", "name", name)
	
	// TODO: Implement actual repository deletion via Soft Serve API
	
	repoPath := filepath.Join(s.dataDir, "repos", name+".git")
	if err := os.RemoveAll(repoPath); err != nil {
		return fmt.Errorf("failed to delete repository: %w", err)
	}
	
	s.logger.Info("Repository deleted successfully", "name", name)
	return nil
}

// GetConnectionInfo returns the SSH connection information for clients
func (s *Server) GetConnectionInfo() ConnectionInfo {
	return ConnectionInfo{
		SSHHost: s.config.Server.Host,
		SSHPort: s.config.Server.Port,
		SSHUrl:  fmt.Sprintf("ssh://%s:%d", s.config.Server.Host, s.config.Server.Port),
	}
}

// ConnectionInfo contains information needed to connect to the git server
type ConnectionInfo struct {
	SSHHost string `json:"ssh_host"`
	SSHPort int    `json:"ssh_port"`  
	SSHUrl  string `json:"ssh_url"`
}