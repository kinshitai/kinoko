package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/kinoko-dev/kinoko/internal/client"
)

var pullAll bool

var pullCmd = &cobra.Command{
	Use:   "pull [skill-repo]",
	Short: "Clone or update skill repositories",
	Long: `Clone or update skill repos from the connected Kinoko server.

  kinoko pull local/fix-nplus1   — clone or update a specific skill
  kinoko pull --all              — sync all cached skills`,
	Args: cobra.MaximumNArgs(1),
	RunE: runPull,
}

func init() {
	pullCmd.Flags().BoolVar(&pullAll, "all", false, "Sync all cached skills")
}

func runPull(cmd *cobra.Command, args []string) error {
	cfg, err := client.LoadLocalConfig(client.DefaultConfigPath())
	if err != nil {
		return fmt.Errorf("not connected to a server (run 'kinoko init --connect <url>' first): %w", err)
	}

	c := client.New(client.ClientConfig{
		APIURL:   cfg.Client.API,
		SSHURL:   cfg.Client.Server,
		CacheDir: cfg.Client.CacheDir,
	})

	if pullAll {
		fmt.Println("🍄 Syncing all cached skills...")
		if err := c.SyncSkills(); err != nil {
			return err
		}
		fmt.Println("✓ All skills synced")
		return nil
	}

	if len(args) == 0 {
		return fmt.Errorf("specify a skill repo or use --all")
	}

	repo := args[0]
	fmt.Printf("🍄 Pulling %s...\n", repo)
	if err := c.CloneSkill(repo, ""); err != nil {
		return err
	}
	fmt.Printf("✓ %s is ready at %s/%s\n", repo, c.CacheDir(), repo)
	return nil
}
