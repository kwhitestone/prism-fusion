package initialize

import (
	"reflect"
	"testing"

	"github.com/kwhitestone/prism-fusion/plugin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type hookOnlyMigrationPlugin struct {
	plugin.BasePlugin
	events []string
}

func (p *hookOnlyMigrationPlugin) BeforeMigrate(*gorm.DB) error {
	p.events = append(p.events, "before")
	return nil
}

func (p *hookOnlyMigrationPlugin) AfterMigrate(*gorm.DB) error {
	p.events = append(p.events, "after")
	return nil
}

func TestMigratePluginRunsHooksWithoutModels(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	candidate := &hookOnlyMigrationPlugin{
		BasePlugin: plugin.BasePlugin{PluginName: "hook-only"},
	}

	if err := migratePlugin(db, candidate); err != nil {
		t.Fatalf("migrate hook-only plugin: %v", err)
	}
	if want := []string{"before", "after"}; !reflect.DeepEqual(candidate.events, want) {
		t.Fatalf("hook events = %v, want %v", candidate.events, want)
	}
}
