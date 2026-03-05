package cmd_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestBuiltBinaryAcceptsPairCommand verifies the compiled binary doesn't crash
// on startup when given the "pair" command. It should start the pairing flow
// (which will eventually fail without a phone), not exit with a panic or
// immediate error.
func TestBuiltBinaryAcceptsPairCommand(t *testing.T) {
	// Build the binary
	tmpDir := t.TempDir()
	binary := filepath.Join(tmpDir, "openmessage")
	build := exec.Command("go", "build", "-o", binary, "..")
	build.Dir = filepath.Join(".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}

	// Run with "pair" — use a temp data dir so it doesn't touch real data
	dataDir := filepath.Join(tmpDir, "data")
	os.MkdirAll(dataDir, 0700)

	cmd := exec.Command(binary, "pair")
	cmd.Env = append(os.Environ(), "OPENMESSAGES_DATA_DIR="+dataDir)

	// Start and give it a moment to initialize
	out := &strings.Builder{}
	cmd.Stdout = out
	cmd.Stderr = out

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start binary: %v", err)
	}

	// Wait briefly then kill — we just want to confirm it doesn't crash immediately
	timer := time.AfterFunc(3*time.Second, func() {
		cmd.Process.Kill()
	})
	defer timer.Stop()

	err := cmd.Wait()
	output := out.String()

	// The process should either still be running (killed by our timer) or
	// have produced some output before failing. A zero-length output + immediate
	// exit means the binary crashed.
	if err != nil && len(output) == 0 {
		t.Errorf("binary exited immediately with no output: %v", err)
	}

	// Should not contain panic traces
	if strings.Contains(output, "panic:") || strings.Contains(output, "runtime error") {
		t.Errorf("binary panicked:\n%s", output)
	}
}

// TestBuiltBinaryRejectsSendGroupNoArgs verifies that running "send-group"
// with no additional arguments exits non-zero and prints usage information.
func TestBuiltBinaryRejectsSendGroupNoArgs(t *testing.T) {
	// Build the binary
	tmpDir := t.TempDir()
	binary := filepath.Join(tmpDir, "openmessage")
	build := exec.Command("go", "build", "-o", binary, "..")
	build.Dir = filepath.Join(".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}

	// Run with "send-group" and no args — use a temp data dir so it doesn't touch real data
	dataDir := filepath.Join(tmpDir, "data")
	os.MkdirAll(dataDir, 0700)

	cmd := exec.Command(binary, "send-group")
	cmd.Env = append(os.Environ(), "OPENMESSAGES_DATA_DIR="+dataDir)

	out, err := cmd.CombinedOutput()
	output := string(out)

	// Should exit with non-zero status
	if err == nil {
		t.Fatal("expected non-zero exit code, but command succeeded")
	}

	// Should contain usage information
	if !strings.Contains(output, "Usage") && !strings.Contains(output, "send-group") {
		t.Errorf("expected output to contain 'Usage' or 'send-group', got:\n%s", output)
	}
}
