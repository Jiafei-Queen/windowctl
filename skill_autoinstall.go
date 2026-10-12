package windowctl

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// skillStampFile sits inside an installed skill and holds the hash of
// the bundle it was copied from, so EnsureBundledSkill can tell a
// current install from a stale one without diffing files.
const skillStampFile = ".windowctl-bundle"

// SkillAutoInstallEnv disables the automatic skill install when set to
// any non-empty value.
const SkillAutoInstallEnv = "WINDOWCTL_NO_SKILL_AUTOINSTALL"

// bundledSkillHash is the sha256 over every embedded skill file (path +
// contents), computed once per process.
func bundledSkillHash() string {
	h := sha256.New()
	_ = fs.WalkDir(bundledSkillFS, "skills", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := bundledSkillFS.ReadFile(path)
		if err != nil {
			return err
		}
		h.Write([]byte(path))
		h.Write([]byte{0})
		h.Write(data)
		return nil
	})
	return hex.EncodeToString(h.Sum(nil))
}

// skillOptOutPath is the marker `windowctl uninstall --skills` leaves so
// the automatic install does not put back a skill the user removed.
func skillOptOutPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "windowctl", "skill-autoinstall-off"), nil
}

func setSkillOptOut(off bool) {
	p, err := skillOptOutPath()
	if err != nil {
		return
	}
	if !off {
		_ = os.Remove(p)
		return
	}
	if os.MkdirAll(filepath.Dir(p), 0o755) == nil {
		_ = os.WriteFile(p, []byte("removed by `windowctl uninstall --skills`; `windowctl install --skills` re-enables\n"), 0o644)
	}
}

// EnsureBundledSkill installs or refreshes the bundled agent skill when
// it is missing or older than this binary, so agents can drive windowctl
// with no manual `install --skills`. Targets ~/.claude/skills always and
// ~/.agents/skills when that directory exists or codex is on PATH. It
// leaves a target alone when it is current, when it is a symlink (a dev
// checkout linked by skills/Taskfile.yml), when the user ran
// `uninstall --skills`, or when WINDOWCTL_NO_SKILL_AUTOINSTALL is set.
// Only installs and updates are returned; callers should treat errors as
// non-fatal.
func EnsureBundledSkill() ([]SkillInstallResult, error) {
	if strings.TrimSpace(os.Getenv(SkillAutoInstallEnv)) != "" {
		return nil, nil
	}
	if p, err := skillOptOutPath(); err == nil {
		if _, err := os.Stat(p); err == nil {
			return nil, nil
		}
	}
	// windowctl is deprecated in favour of aos, which ships the one agent
	// skill. Nothing new is installed; once aos is on PATH the copies this
	// binary installed (stamped) are removed so two skills never compete.
	if _, err := exec.LookPath("aos"); err != nil {
		return nil, nil
	}
	return retireBundledSkill(SkillInstallOptions{IncludeAgents: true})
}

// retireBundledSkill removes stamped (auto-installed) copies of the
// window-ctl skill. Symlinked dev copies and hand-made ones are kept.
func retireBundledSkill(opts SkillInstallOptions) ([]SkillInstallResult, error) {
	targets, err := skillInstallTargets(opts)
	if err != nil {
		return nil, err
	}
	var out []SkillInstallResult
	var errs []error
	for _, t := range targets {
		if fi, err := os.Lstat(t.path); err != nil || fi.Mode()&os.ModeSymlink != 0 {
			continue
		}
		if _, err := os.Stat(filepath.Join(t.path, skillStampFile)); err != nil {
			continue
		}
		if err := os.RemoveAll(t.path); err != nil {
			errs = append(errs, err)
			continue
		}
		out = append(out, SkillInstallResult{Host: t.host, Path: t.path, Action: "removed (superseded by aos)"})
	}
	return out, errors.Join(errs...)
}

func wantAgentsSkill() bool {
	if _, err := exec.LookPath("codex"); err == nil {
		return true
	}
	root, err := skillRootDir("", "AGENTS_SKILLS_DIR", ".agents")
	if err != nil {
		return false
	}
	if os.Getenv("AGENTS_SKILLS_DIR") != "" {
		root = filepath.Dir(root)
	}
	_, err = os.Stat(root)
	return err == nil
}

func ensureBundledSkill(opts SkillInstallOptions) ([]SkillInstallResult, error) {
	targets, err := skillInstallTargets(opts)
	if err != nil {
		return nil, err
	}
	want := bundledSkillHash()
	var out []SkillInstallResult
	var errs []error
	for _, t := range targets {
		if fi, err := os.Lstat(t.path); err == nil && fi.Mode()&os.ModeSymlink != 0 {
			continue
		}
		if got, err := os.ReadFile(filepath.Join(t.path, skillStampFile)); err == nil && strings.TrimSpace(string(got)) == want {
			continue
		}
		action, err := installBundledSkillAt(t.path)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		out = append(out, SkillInstallResult{Host: t.host, Path: t.path, Action: action})
	}
	return out, errors.Join(errs...)
}

// SetSkillAutoInstall records the user's choice: false (after
// `uninstall --skills`) stops EnsureBundledSkill from reinstalling;
// true (after `install --skills`) re-enables it.
func SetSkillAutoInstall(on bool) { setSkillOptOut(!on) }
