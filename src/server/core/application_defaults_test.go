package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/kwhitestone/prism-fusion/global"
	"go.uber.org/zap"
)

func TestDefaultApplicationInfrastructure(t *testing.T) {
	oldConfig, oldVP, oldDB, oldLog := global.PRISM_CONFIG, global.PRISM_VP, global.PRISM_DB, global.PRISM_LOG
	t.Cleanup(func() {
		global.PRISM_CONFIG, global.PRISM_VP, global.PRISM_DB, global.PRISM_LOG = oldConfig, oldVP, oldDB, oldLog
		if oldLog != nil {
			zap.ReplaceGlobals(oldLog)
		} else {
			zap.ReplaceGlobals(zap.NewNop())
		}
	})
	path := filepath.Join(t.TempDir(), "config.yaml")
	contents := fmt.Sprintf("system:\n  addr: 26639\nsqlite:\n  path: %q\n  max-idle-conns: 1\n  max-open-conns: 1\nzap:\n  director: %q\n  level: info\n  format: json\n", filepath.Join(t.TempDir(), "app.db"), t.TempDir())
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
	ops := defaultApplicationOperations(ApplicationOptions{ConfigPath: path})
	if err := ops.configure(); err != nil {
		t.Fatal(err)
	}
	if global.PRISM_DB != nil {
		t.Fatal("configuration opened the database early")
	}
	database, err := ops.openDatabase()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := ops.lifecycle.Prepare(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer ops.lifecycle.Stop(context.Background())
	if err := ops.migrate(); err != nil {
		t.Fatal(err)
	}
	if _, err := ops.router(); err != nil {
		t.Fatal(err)
	}
	if ops.address() != ":26639" {
		t.Fatalf("address = %s", ops.address())
	}
	global.PRISM_CONFIG.Sqlite.Path = ""
	if _, err := ops.openDatabase(); err == nil {
		t.Fatal("missing database was accepted")
	}
}

func TestApplicationPublicEntryRejectsRepeatedAttempt(t *testing.T) {
	// The first configuration failure must still leave the process host closed
	// to retries: global/plugin state must never be reused as a second host.
	if err := RunApplication(ApplicationOptions{ConfigPath: filepath.Join(t.TempDir(), "missing.yaml")}); err == nil {
		t.Fatal("missing config did not fail")
	}
	if err := RunApplicationContext(context.Background(), ApplicationOptions{}); !errors.Is(err, ErrApplicationState) {
		t.Fatalf("repeated host error = %v", err)
	}
}
