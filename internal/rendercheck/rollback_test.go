package rendercheck

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The rollback script (0079), run against a directory of its own rather
// than an AP: a bundle that never confirmed itself gets the files it
// replaced put back, the rollback recorded, and the service restarted; a
// confirmed update, or a trial for another bundle, is left alone.
func TestRollback(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh")
	}
	script, err := filepath.Abs(filepath.Join(agentDir, "files", "usr", "libexec", "aeolus-rollback"))
	if err != nil {
		t.Fatal(err)
	}
	const hash = "4fa1c2e0b3d5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f"
	const other = "0000000000000000000000000000000000000000000000000000000000000000"

	// set lays out an AP: the agent now (the new one, on trial for trial,
	// unless that is empty), and its backed-up old copy.
	set := func(t *testing.T, trial string) (root, dir string) {
		root = t.TempDir()
		dir = filepath.Join(root, "etc", "aeolus")
		for p, text := range map[string]string{
			filepath.Join(root, "usr", "sbin", "aeolus-agent"):                               "the new agent",
			filepath.Join(dir, "agent.prev", "usr", "sbin", "aeolus-agent"):                  "the old agent",
			filepath.Join(root, "usr", "share", "ucode", "aeolus", "render.uc"):              "the new renderer",
			filepath.Join(dir, "agent.prev", "usr", "share", "ucode", "aeolus", "render.uc"): "the old renderer",
		} {
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if trial != "" {
			marker := "hash=" + trial + "\nversion=v9.9.9-broken\n"
			if err := os.WriteFile(filepath.Join(dir, "agent.trial"), []byte(marker), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return root, dir
	}
	run := func(t *testing.T, root, dir string) {
		cmd := exec.Command(sh, script, hash, "0")
		cmd.Env = append(os.Environ(), "AEOLUS_DIR="+dir, "AEOLUS_ROOT="+root,
			"AEOLUS_RESTART=touch "+filepath.Join(root, "restarted"))
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
	}
	// runProcd runs it as on an AP, with no restart of its own given: a
	// stand-in ubus on PATH says what it was asked.
	runProcd := func(t *testing.T, root, dir string) string {
		bin := filepath.Join(root, "bin")
		os.MkdirAll(bin, 0o755)
		log := filepath.Join(root, "ubus.log")
		// Forward slashes, quoted: sh would take a Windows path's
		// backslashes for escapes.
		os.WriteFile(filepath.Join(bin, "ubus"), []byte("#!/bin/sh\necho \"$*\" >>'"+filepath.ToSlash(log)+"'\n"), 0o755)
		cmd := exec.Command(sh, script, hash, "0")
		cmd.Env = append(os.Environ(), "AEOLUS_DIR="+dir, "AEOLUS_ROOT="+root, "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		b, _ := os.ReadFile(log)
		return string(b)
	}
	read := func(p string) string {
		b, _ := os.ReadFile(p)
		return string(b)
	}
	exists := func(p string) bool {
		_, err := os.Stat(p)
		return err == nil
	}

	// The restart goes through rpcd's rc object: started by the agent, the
	// script is in its service, whose processes a restart kills on OpenWrt
	// main, the init script it would run among them.
	t.Run("restarts through procd", func(t *testing.T) {
		root, dir := set(t, hash)
		if got := runProcd(t, root, dir); !strings.Contains(got, `call rc init {"name":"aeolus","action":"restart"}`) {
			t.Errorf("procd was asked: %q", got)
		}
	})

	t.Run("not confirmed", func(t *testing.T) {
		root, dir := set(t, hash)
		run(t, root, dir)
		if got := read(filepath.Join(root, "usr", "sbin", "aeolus-agent")); got != "the old agent" {
			t.Errorf("the agent is %q", got)
		}
		if got := read(filepath.Join(root, "usr", "share", "ucode", "aeolus", "render.uc")); got != "the old renderer" {
			t.Errorf("the renderer is %q", got)
		}
		result := read(filepath.Join(dir, "agent.result"))
		for _, want := range []string{`"hash":"` + hash + `"`, `"version":"v9.9.9-broken"`, `"state":"rolled-back"`} {
			if !strings.Contains(result, want) {
				t.Errorf("result %q lacks %s", result, want)
			}
		}
		if exists(filepath.Join(dir, "agent.trial")) {
			t.Error("the trial marker is still there")
		}
		if !exists(filepath.Join(root, "restarted")) {
			t.Error("the service wasn't restarted")
		}
	})

	for name, trial := range map[string]string{"confirmed": "", "another bundle's trial": other} {
		t.Run(name, func(t *testing.T) {
			root, dir := set(t, trial)
			run(t, root, dir)
			if got := read(filepath.Join(root, "usr", "sbin", "aeolus-agent")); got != "the new agent" {
				t.Errorf("the agent is %q", got)
			}
			if exists(filepath.Join(dir, "agent.result")) || exists(filepath.Join(root, "restarted")) {
				t.Error("rolled back anyway")
			}
		})
	}
}
