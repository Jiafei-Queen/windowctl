package windowctl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallBundledSkillCopiesBundleToClaudeDir(t *testing.T) {
	claudeDir := t.TempDir()

	results, err := InstallBundledSkill(SkillInstallOptions{ClaudeDir: claudeDir})
	if err != nil {
		t.Fatalf("InstallBundledSkill: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 install result, got %d", len(results))
	}
	if results[0].Host != "claude" || results[0].Action != "installed" {
		t.Fatalf("unexpected result: %+v", results[0])
	}

	skillDir := filepath.Join(claudeDir, BundledSkillName)
	for _, rel := range []string{"SKILL.md", "references/move.md", "README.md"} {
		if _, err := os.Stat(filepath.Join(skillDir, rel)); err != nil {
			t.Fatalf("expected installed file %s: %v", rel, err)
		}
	}

	data, err := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		t.Fatalf("read installed SKILL.md: %v", err)
	}
	if !strings.Contains(string(data), "name: "+BundledSkillName) {
		t.Fatalf("installed SKILL.md missing bundled skill name, got:\n%s", string(data))
	}
}

func TestInstallBundledSkillSecondRunReportsUpdate(t *testing.T) {
	claudeDir := t.TempDir()

	if _, err := InstallBundledSkill(SkillInstallOptions{ClaudeDir: claudeDir}); err != nil {
		t.Fatalf("first InstallBundledSkill: %v", err)
	}
	results, err := InstallBundledSkill(SkillInstallOptions{ClaudeDir: claudeDir})
	if err != nil {
		t.Fatalf("second InstallBundledSkill: %v", err)
	}
	if results[0].Action != "updated" {
		t.Fatalf("expected update action on second install, got %+v", results[0])
	}
}

func TestUninstallBundledSkillRemovesClaudeAndAgentsInstalls(t *testing.T) {
	claudeDir := t.TempDir()
	agentsDir := t.TempDir()
	opts := SkillInstallOptions{
		ClaudeDir:     claudeDir,
		AgentsDir:     agentsDir,
		IncludeAgents: true,
	}

	if _, err := InstallBundledSkill(opts); err != nil {
		t.Fatalf("InstallBundledSkill: %v", err)
	}

	results, err := UninstallBundledSkill(opts)
	if err != nil {
		t.Fatalf("UninstallBundledSkill: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 uninstall results, got %d", len(results))
	}
	for _, result := range results {
		if result.Action != "removed" {
			t.Fatalf("expected removed action, got %+v", result)
		}
		if _, err := os.Lstat(result.Path); !os.IsNotExist(err) {
			t.Fatalf("expected %s to be removed, stat err=%v", result.Path, err)
		}
	}

	results, err = UninstallBundledSkill(opts)
	if err != nil {
		t.Fatalf("second UninstallBundledSkill: %v", err)
	}
	for _, result := range results {
		if result.Action != "not-installed" {
			t.Fatalf("expected not-installed on second uninstall, got %+v", result)
		}
	}
}

func TestEnsureBundledSkillInstallsOnceThenSkips(t *testing.T) {
	dir := t.TempDir()
	opts := SkillInstallOptions{ClaudeDir: dir}
	res, err := ensureBundledSkill(opts)
	if err != nil || len(res) != 1 || res[0].Action != "installed" {
		t.Fatalf("first run: %+v %v", res, err)
	}
	if res, err = ensureBundledSkill(opts); err != nil || len(res) != 0 {
		t.Fatalf("second run should be a no-op: %+v %v", res, err)
	}
	stamp := filepath.Join(dir, BundledSkillName, skillStampFile)
	if err := os.WriteFile(stamp, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if res, err = ensureBundledSkill(opts); err != nil || len(res) != 1 || res[0].Action != "updated" {
		t.Fatalf("stale stamp should update: %+v %v", res, err)
	}
}

func TestEnsureBundledSkillLeavesSymlinkAlone(t *testing.T) {
	dir := t.TempDir()
	src := t.TempDir()
	if err := os.Symlink(src, filepath.Join(dir, BundledSkillName)); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	res, err := ensureBundledSkill(SkillInstallOptions{ClaudeDir: dir})
	if err != nil || len(res) != 0 {
		t.Fatalf("symlinked dev install must not be replaced: %+v %v", res, err)
	}
}
