package windowctl

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const BundledSkillName = "window-ctl-skill"

type SkillInstallOptions struct {
	ClaudeDir     string
	AgentsDir     string
	IncludeAgents bool
}

type SkillInstallResult struct {
	Host   string
	Path   string
	Action string
}

//go:embed skills
var bundledSkillFS embed.FS

type skillInstallTarget struct {
	host string
	path string
}

func InstallBundledSkill(opts SkillInstallOptions) ([]SkillInstallResult, error) {
	targets, err := skillInstallTargets(opts)
	if err != nil {
		return nil, err
	}
	results := make([]SkillInstallResult, 0, len(targets))
	for _, target := range targets {
		action, err := installBundledSkillAt(target.path)
		if err != nil {
			return nil, fmt.Errorf("%s install failed: %w", target.host, err)
		}
		results = append(results, SkillInstallResult{
			Host:   target.host,
			Path:   target.path,
			Action: action,
		})
	}
	return results, nil
}

func UninstallBundledSkill(opts SkillInstallOptions) ([]SkillInstallResult, error) {
	targets, err := skillInstallTargets(opts)
	if err != nil {
		return nil, err
	}
	results := make([]SkillInstallResult, 0, len(targets))
	for _, target := range targets {
		action, err := uninstallBundledSkillAt(target.path)
		if err != nil {
			return nil, fmt.Errorf("%s uninstall failed: %w", target.host, err)
		}
		results = append(results, SkillInstallResult{
			Host:   target.host,
			Path:   target.path,
			Action: action,
		})
	}
	return results, nil
}

func skillInstallTargets(opts SkillInstallOptions) ([]skillInstallTarget, error) {
	claudeRoot, err := skillRootDir(opts.ClaudeDir, "CLAUDE_SKILLS_DIR", ".claude", "skills")
	if err != nil {
		return nil, err
	}
	targets := []skillInstallTarget{{
		host: "claude",
		path: filepath.Join(claudeRoot, BundledSkillName),
	}}
	if opts.IncludeAgents {
		agentsRoot, err := skillRootDir(opts.AgentsDir, "AGENTS_SKILLS_DIR", ".agents", "skills")
		if err != nil {
			return nil, err
		}
		targets = append(targets, skillInstallTarget{
			host: "agents",
			path: filepath.Join(agentsRoot, BundledSkillName),
		})
	}
	return targets, nil
}

func skillRootDir(explicit, envKey string, fallback ...string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	if envDir := strings.TrimSpace(os.Getenv(envKey)); envDir != "" {
		return envDir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory for %s: %w", envKey, err)
	}
	parts := append([]string{home}, fallback...)
	return filepath.Join(parts...), nil
}

func installBundledSkillAt(dst string) (string, error) {
	action := "installed"
	if _, err := os.Lstat(dst); err == nil {
		action = "updated"
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("stat %s: %w", dst, err)
	}

	parent := filepath.Dir(dst)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", fmt.Errorf("create parent %s: %w", parent, err)
	}

	tmp, err := os.MkdirTemp(parent, BundledSkillName+".tmp-*")
	if err != nil {
		return "", fmt.Errorf("create temp install dir in %s: %w", parent, err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(tmp)
		}
	}()

	if err := copyBundledSkillTree(tmp); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(tmp, skillStampFile), []byte(bundledSkillHash()+"\n"), 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", skillStampFile, err)
	}
	if err := os.RemoveAll(dst); err != nil {
		return "", fmt.Errorf("remove existing install %s: %w", dst, err)
	}
	if err := os.Rename(tmp, dst); err != nil {
		return "", fmt.Errorf("activate install at %s: %w", dst, err)
	}
	cleanup = false
	return action, nil
}

func uninstallBundledSkillAt(dst string) (string, error) {
	if _, err := os.Lstat(dst); os.IsNotExist(err) {
		return "not-installed", nil
	} else if err != nil {
		return "", fmt.Errorf("stat %s: %w", dst, err)
	}
	if err := os.RemoveAll(dst); err != nil {
		return "", fmt.Errorf("remove %s: %w", dst, err)
	}
	return "removed", nil
}

func copyBundledSkillTree(dst string) error {
	return fs.WalkDir(bundledSkillFS, "skills", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == "skills" {
			return nil
		}
		rel := strings.TrimPrefix(path, "skills/")
		outPath := filepath.Join(dst, filepath.FromSlash(rel))
		if d.IsDir() {
			return os.MkdirAll(outPath, 0o755)
		}
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			return fmt.Errorf("create parent for %s: %w", outPath, err)
		}
		data, err := bundledSkillFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read embedded skill file %s: %w", path, err)
		}
		if err := os.WriteFile(outPath, data, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", outPath, err)
		}
		return nil
	})
}
