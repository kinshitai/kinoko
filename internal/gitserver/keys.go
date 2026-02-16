package gitserver

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const (
	adminKeyName      = "kinoko_admin_ed25519"
	adminKeyPubSuffix = ".pub"
	adminKeyPerms     = 0600
	adminKeyPubPerms  = 0644
)

// ensureAdminKeys generates an ed25519 keypair for admin access if one doesn't exist
// Returns the path to the private key file
func (s *Server) ensureAdminKeys() (string, error) {
	privateKeyPath := filepath.Join(s.dataDir, adminKeyName)
	publicKeyPath := privateKeyPath + adminKeyPubSuffix

	// Check if keys already exist
	if _, err := os.Stat(privateKeyPath); err == nil {
		if _, err := os.Stat(publicKeyPath); err == nil {
			s.logger.Debug("Admin SSH keys already exist", "private", privateKeyPath, "public", publicKeyPath)
			return privateKeyPath, nil
		}
	}

	s.logger.Info("Generating admin SSH keypair", "private", privateKeyPath, "public", publicKeyPath)

	// Use ssh-keygen to generate the keypair
	cmd := exec.Command("ssh-keygen",
		"-t", "ed25519",
		"-f", privateKeyPath,
		"-N", "", // No passphrase
		"-C", "kinoko-admin", // Comment
	)

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to generate SSH keys with ssh-keygen: %w", err)
	}

	// Set proper permissions
	if err := os.Chmod(privateKeyPath, adminKeyPerms); err != nil {
		return "", fmt.Errorf("failed to set private key permissions: %w", err)
	}

	if err := os.Chmod(publicKeyPath, adminKeyPubPerms); err != nil {
		return "", fmt.Errorf("failed to set public key permissions: %w", err)
	}

	s.logger.Info("Admin SSH keypair generated successfully")
	return privateKeyPath, nil
}

// getAdminPublicKey reads and returns the admin public key
func (s *Server) getAdminPublicKey() (string, error) {
	publicKeyPath := filepath.Join(s.dataDir, adminKeyName+adminKeyPubSuffix)

	data, err := os.ReadFile(publicKeyPath)
	if err != nil {
		return "", fmt.Errorf("failed to read admin public key: %w", err)
	}

	// Return the key data without the newline
	keyData := string(data)
	if len(keyData) > 0 && keyData[len(keyData)-1] == '\n' {
		keyData = keyData[:len(keyData)-1]
	}

	return keyData, nil
}
