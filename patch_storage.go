package main

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type patchInput struct {
	data, clean []byte
	layout      *patchLayout
	baseline    string
}

type patchManifest struct {
	Backup      string `json:"backup"`
	OriginalSHA string `json:"original_sha256"`
	PatchedSHA  string `json:"patched_sha256"`
}

// A clean image is the source of build-specific CALL bytes. Already patched
// images require a matching backup; bytes outside all 19 targets must be identical.
func loadPatchInput(path string) (*patchInput, error) {
	data, err := readLimited(path, 512<<20)
	if err != nil {
		return nil, err
	}
	layout, scanErr := resolvePatchLayout(data)
	if scanErr == nil {
		return &patchInput{data, data, layout, path}, nil
	}
	// Only missing signatures may be caused by an already-patched image.
	// Internal definition errors and ambiguous matches must be reported directly.
	var matchErr *signatureMatchError
	if !errors.As(scanErr, &matchErr) || matchErr.count != 0 {
		return nil, scanErr
	}
	manifestData, err := readLimited(path+".dimraeth_patch.json", 1<<20)
	if err == nil {
		var m patchManifest
		if json.Unmarshal(manifestData, &m) != nil || m.Backup == "" || filepath.Base(m.Backup) != m.Backup || m.Backup == "." || m.Backup == ".." || strings.ContainsAny(m.Backup, `/\`) || len(m.OriginalSHA) != 64 {
			return nil, fmt.Errorf("Invalid patch backup manifest. No DLL modifications were written")
		}
		backupPath := filepath.Join(filepath.Dir(path), m.Backup)
		clean, err := readLimited(backupPath, 512<<20)
		if err != nil {
			return nil, fmt.Errorf("Cannot read patch backup: %w", err)
		}
		if fmt.Sprintf("%x", sha256.Sum256(clean)) != m.OriginalSHA {
			return nil, fmt.Errorf("Backup SHA-256 does not match manifest. No DLL modifications were written")
		}
		layout, err := resolvePatchLayout(clean)
		if err != nil {
			return nil, err
		}
		if err = layout.validateCurrent(data); err != nil {
			return nil, err
		}
		return &patchInput{data, clean, layout, backupPath}, nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}

	// Adopt backups from earlier desktop releases and the standalone patchers.
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	var chosen *patchInput
	seen := map[[32]byte]bool{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || (!strings.HasPrefix(name, filepath.Base(path)+".original_") && !strings.HasPrefix(name, filepath.Base(path)+".backup-")) {
			continue
		}
		backupPath := filepath.Join(filepath.Dir(path), name)
		clean, err := readLimited(backupPath, 512<<20)
		if err != nil || len(clean) != len(data) {
			continue
		}
		digest := sha256.Sum256(clean)
		if seen[digest] {
			continue
		}
		seen[digest] = true
		layout, err := resolvePatchLayout(clean)
		if err != nil || layout.validateCurrent(data) != nil {
			continue
		}
		if chosen != nil {
			return nil, fmt.Errorf("Multiple different clean backups match the patched DLL. Restore the correct clean DLL before patching. No DLL modifications were written")
		}
		chosen = &patchInput{data, clean, layout, backupPath}
	}
	if chosen != nil {
		return chosen, nil
	}
	return nil, fmt.Errorf("%w\nIf this DLL was already patched, keep its matching clean .original_* or .backup-* file beside it, or restore a clean DLL first", scanErr)
}

func writeNewBackup(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if os.IsExist(err) {
		prior, readErr := readLimited(path, 512<<20)
		if readErr != nil {
			return readErr
		}
		if sha256.Sum256(prior) != sha256.Sum256(data) {
			return fmt.Errorf("Backup already exists with different contents: %s", path)
		}
		return nil
	}
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(path)
	}
	return err
}

func writePatchManifest(path string, m patchManifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".dimraeth-manifest-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return replaceFile(f.Name(), path)
}

func patchGameFile(path string, multiplier float64, rarity, mode, stars int, settings progressionSettings) (string, error) {
	input, err := loadPatchInput(path)
	if err != nil {
		return "", err
	}
	// Resolve and verify every target, reject overlaps, and plan all output in
	// memory before creating backups or modifying the installation.
	out, err := input.layout.patchedGameDLL(input.data, multiplier, rarity, mode, stars, settings)
	if err != nil {
		return "", err
	}
	cleanSHA := fmt.Sprintf("%x", input.layout.cleanHash)
	cleanBackup := path + ".original_" + cleanSHA[:8]
	if err = writeNewBackup(cleanBackup, input.clean); err != nil {
		return "", err
	}
	rollback, err := backup(path)
	if err != nil {
		return "", err
	}
	manifest := patchManifest{filepath.Base(cleanBackup), cleanSHA, fmt.Sprintf("%x", sha256.Sum256(out))}
	// Write recovery metadata first: even a failed replacement keeps the clean
	// backup discoverable, and a manifest failure leaves the DLL untouched.
	if err = writePatchManifest(path+".dimraeth_patch.json", manifest); err != nil {
		return "", err
	}
	return rollback, replaceChecked(path, out, sha256.Sum256(input.data))
}

func scanPatchFile(path string) ([]string, error) {
	input, err := loadPatchInput(path)
	if err != nil {
		return nil, err
	}
	// Match the requested full --scan-only command: default level 50, loot x3,
	// Ancient, stars 3, and all progression and RNG bypasses enabled.
	settings := progressionSettings{EditableAttributes: true, NoAttributeCap: true, EditableSkillPoints: true, MaxLevel: 50}
	if _, err = input.layout.patchedGameDLL(input.data, 3, 5, rngEnable, 3, settings); err != nil {
		return nil, err
	}
	rows := []string{
		fmt.Sprintf("Verified %d patch targets. Scan only: no files were written.", len(input.layout.targets)),
		"Full scan: level 50, loot x3, Ancient, stars 3, all progression and RNG patches.",
		fmt.Sprintf("Input SHA-256 (informational): %x", sha256.Sum256(input.data)),
		"Signature source: " + input.baseline,
	}
	for _, s := range input.layout.sections {
		rows = append(rows, fmt.Sprintf("Executable section: %s [0x%X:0x%X]", s.name, s.off, uint64(s.off)+uint64(s.raw)))
	}
	for _, p := range input.layout.targets {
		rows = append(rows, fmt.Sprintf("[FOUND] %s  |  %s  |  file=0x%X  VA=0x%X", p.name, p.section, p.offset, p.va))
	}
	return rows, nil
}
