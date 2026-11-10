package sourcetest

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Elixir uses the installed interpreter or provisions an explicitly selected
// offline container before parser calls start their own shorter timeout.
func Elixir(t *testing.T, fixtures string) {
	t.Helper()
	if _, err := exec.LookPath("elixir"); err == nil {
		return
	}
	image := os.Getenv("DEPTRUST_TEST_ELIXIR_IMAGE")
	if image == "" {
		t.Skip("Elixir or an explicitly supplied cached official test image is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "run", "--detach", "--rm", "--pull=never", "--network=none", "--read-only", "--tmpfs", "/tmp", "--mount", "type=bind,source="+fixtures+",target="+fixtures+",readonly", image, "sleep", "infinity")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("start offline Elixir test container: %v", err)
	}
	id := strings.TrimSpace(string(out))
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		if out, err := exec.CommandContext(cleanupCtx, "docker", "rm", "--force", id).CombinedOutput(); err != nil {
			t.Errorf("remove Elixir test container: %v %s", err, out)
		}
	})
	tools := t.TempDir()
	wrapper := "#!/bin/sh\nexec docker exec \"$DEPTRUST_TEST_ELIXIR_CONTAINER\" elixir \"$@\"\n"
	if err := os.WriteFile(filepath.Join(tools, "elixir"), []byte(wrapper), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DEPTRUST_TEST_ELIXIR_CONTAINER", id)
	t.Setenv("PATH", tools+string(os.PathListSeparator)+os.Getenv("PATH"))
}
