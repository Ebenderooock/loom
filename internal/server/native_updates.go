package server

import (
	"github.com/ebenderooock/loom/internal/kernel/config"
	"github.com/ebenderooock/loom/internal/updater"
)

func nativeBinding(cfg *config.Config) updater.Binding {
	return updater.Binding{ConfigDir: cfg.ConfigDir, DataDir: cfg.DataDir,
		SQLitePath: cfg.Storage.SQLite.Path, Engine: cfg.Storage.Engine, Listen: cfg.HTTP.Addr}
}
