package cache

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/riyanpratamap/flash-cashback/backend/internal/config"
)

// The go-redis library writes failed dials, with the address, to its own
// logger on stderr. The test dials a closed port in a child process (the
// logger is process-wide) and reads what it printed.
func TestNewClientSilencesDriverLogger(t *testing.T) {
	if os.Getenv("FC_DIAL_CHILD") == "1" {
		c := NewClient(config.Config{RedisAddr: "127.0.0.1:1", RedisTimeout: 200 * time.Millisecond})
		defer c.Close()
		_ = Ping(context.Background(), c)
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestNewClientSilencesDriverLogger$")
	cmd.Env = append(os.Environ(), "FC_DIAL_CHILD=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("child: %v\n%s", err, out)
	}
	if strings.Contains(string(out), "127.0.0.1") || strings.Contains(string(out), "redis:") {
		t.Fatalf("the driver logged the address:\n%s", out)
	}
}
