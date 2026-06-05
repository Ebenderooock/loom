// Package backup provides backup and restore for Loom's SQLite database and config file.
package backup

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// CreateBackup creates a .tar.gz containing the SQLite database (data/loom.db)
// and the config file (loom.json). If outputPath is empty a timestamped file is
// created under {configDir}/backups/.
func CreateBackup(configDir string, outputPath string) (string, error) {
	dbPath := filepath.Join(configDir, "data", "loom.db")
	cfgPath := filepath.Join(configDir, "loom.json")

	// Validate source files exist.
	for _, p := range []string{dbPath, cfgPath} {
		if _, err := os.Stat(p); err != nil {
			return "", fmt.Errorf("source file missing: %w", err)
		}
	}

	if outputPath == "" {
		backupDir := filepath.Join(configDir, "backups")
		if err := os.MkdirAll(backupDir, 0o755); err != nil {
			return "", fmt.Errorf("create backups dir: %w", err)
		}
		ts := time.Now().UTC().Format("20060102-150405")
		outputPath = filepath.Join(backupDir, fmt.Sprintf("loom-backup-%s.tar.gz", ts))
	} else {
		if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
			return "", fmt.Errorf("create output dir: %w", err)
		}
	}

	outFile, err := os.Create(outputPath)
	if err != nil {
		return "", fmt.Errorf("create backup file: %w", err)
	}
	defer outFile.Close()

	gw := gzip.NewWriter(outFile)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()

	// Archive entries with paths relative to configDir.
	entries := []struct {
		diskPath string
		arcName  string
	}{
		{dbPath, "data/loom.db"},
		{cfgPath, "loom.json"},
	}

	for _, e := range entries {
		if err := addFileToTar(tw, e.diskPath, e.arcName); err != nil {
			return "", fmt.Errorf("archive %s: %w", e.arcName, err)
		}
	}

	return outputPath, nil
}

func addFileToTar(tw *tar.Writer, diskPath, arcName string) error {
	f, err := os.Open(diskPath)
	if err != nil {
		return err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return err
	}

	hdr := &tar.Header{
		Name:    arcName,
		Size:    info.Size(),
		Mode:    int64(info.Mode()),
		ModTime: info.ModTime(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err = io.Copy(tw, f)
	return err
}

// RestoreBackup extracts a backup tarball, replacing the SQLite database and
// config file. Existing files are renamed to .bak before overwriting.
// The loom server should NOT be running during restore.
func RestoreBackup(configDir string, backupPath string) error {
	f, err := os.Open(backupPath)
	if err != nil {
		return fmt.Errorf("open backup: %w", err)
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("gzip reader: %w", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)

	allowed := map[string]bool{
		"data/loom.db": true,
		"loom.json":    true,
	}

	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar: %w", err)
		}

		if !allowed[hdr.Name] {
			continue
		}

		destPath := filepath.Join(configDir, hdr.Name)

		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			return fmt.Errorf("create dir for %s: %w", hdr.Name, err)
		}

		// Back up existing file.
		if _, err := os.Stat(destPath); err == nil {
			bakPath := destPath + ".bak"
			if err := os.Rename(destPath, bakPath); err != nil {
				return fmt.Errorf("backup existing %s: %w", hdr.Name, err)
			}
		}

		out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
		if err != nil {
			return fmt.Errorf("create %s: %w", hdr.Name, err)
		}
		if _, err := io.Copy(out, tr); err != nil {
			out.Close()
			return fmt.Errorf("write %s: %w", hdr.Name, err)
		}
		out.Close()
	}

	return nil
}
