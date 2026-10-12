package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"

	windowctl "github.com/muthuishere/windowctl"
)

type skillInstallFn func(windowctl.SkillInstallOptions) ([]windowctl.SkillInstallResult, error)

func installCmd(args []string) {
	rc := runInstall(os.Stdout, os.Stderr, args, exec.LookPath, windowctl.InstallBundledSkill)
	if rc == 0 {
		windowctl.SetSkillAutoInstall(true)
	}
	if rc != 0 {
		os.Exit(rc)
	}
}

func uninstallCmd(args []string) {
	rc := runUninstall(os.Stdout, os.Stderr, args, exec.LookPath, windowctl.UninstallBundledSkill)
	if rc == 0 {
		windowctl.SetSkillAutoInstall(false)
	}
	if rc != 0 {
		os.Exit(rc)
	}
}

func runInstall(stdout, stderr io.Writer, args []string, lookPath func(string) (string, error), installFn skillInstallFn) int {
	return runTopLevelSkillAction(stdout, stderr, "install", args, lookPath, installFn)
}

func runUninstall(stdout, stderr io.Writer, args []string, lookPath func(string) (string, error), uninstallFn skillInstallFn) int {
	return runTopLevelSkillAction(stdout, stderr, "uninstall", args, lookPath, uninstallFn)
}

func runTopLevelSkillAction(stdout, stderr io.Writer, verb string, args []string, lookPath func(string) (string, error), actionFn skillInstallFn) int {
	fs := flag.NewFlagSet(verb, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	skills := fs.Bool("skills", false, "install or remove the bundled window-ctl skill")
	agents := fs.Bool("agents", false, "also target ~/.agents/skills even when codex is not on PATH")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printTopLevelSkillUsage(stdout, verb)
			return 0
		}
		fmt.Fprintf(stderr, "windowctl %s: %v\n\n", verb, err)
		printTopLevelSkillUsage(stderr, verb)
		return 2
	}
	if !*skills {
		fmt.Fprintf(stderr, "windowctl %s: --skills is required\n\n", verb)
		printTopLevelSkillUsage(stderr, verb)
		return 2
	}
	if len(fs.Args()) != 0 {
		fmt.Fprintf(stderr, "windowctl %s: unexpected arguments\n\n", verb)
		printTopLevelSkillUsage(stderr, verb)
		return 2
	}
	return executeSkillAction(stdout, stderr, verb, *agents, lookPath, actionFn)
}

func executeSkillAction(stdout, stderr io.Writer, label string, agents bool, lookPath func(string) (string, error), actionFn skillInstallFn) int {
	results, err := actionFn(windowctl.SkillInstallOptions{IncludeAgents: resolveIncludeAgents(agents, lookPath)})
	if err != nil {
		fmt.Fprintf(stderr, "windowctl %s: %v\n", label, err)
		return 1
	}
	for _, result := range results {
		fmt.Fprintf(stdout, "%s: %s at %s\n", result.Host, result.Action, result.Path)
	}
	return 0
}

func resolveIncludeAgents(agents bool, lookPath func(string) (string, error)) bool {
	if agents {
		return true
	}
	_, err := lookPath("codex")
	return err == nil
}

func printTopLevelSkillUsage(w io.Writer, verb string) {
	fmt.Fprintf(w, "usage: windowctl %s --skills [--agents]\n", verb)
}

// autoInstallSkill keeps the bundled agent skill installed and current
// on every run, so `install --skills` is never a required step. Silent
// unless it changed something; never fails the command.
func autoInstallSkill(cmd string) {
	switch cmd {
	case "install", "uninstall", "help", "-h", "--help":
		return
	}
	results, _ := windowctl.EnsureBundledSkill()
	for _, r := range results {
		fmt.Fprintf(os.Stderr, "windowctl: agent skill %s at %s (set %s=1 to disable)\n", r.Action, r.Path, windowctl.SkillAutoInstallEnv)
	}
}
