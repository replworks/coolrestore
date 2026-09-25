package restore

import (
	"fmt"
	"os"
	"path/filepath"
)

// ApplyReplace atomically installs the already-materialized staging directory
// as the target directory. The plan is used as the authorization boundary;
// the staged tree is the complete resulting content.
func ApplyReplace(stagingDir, targetDir string, plan Plan) error {
	if plan.Mode != ModeReplace {
		return fmt.Errorf("cannot apply %s plan as replace", plan.Mode)
	}
	if err := validateDirectory(stagingDir, "staging"); err != nil {
		return err
	}
	if err := validateTarget(targetDir); err != nil {
		return err
	}

	parent := filepath.Dir(targetDir)
	if err := requireSameFilesystem(stagingDir, parent); err != nil {
		return fmt.Errorf("replace requires staging and target on the same filesystem: %w", err)
	}

	targetExists := false
	if _, err := os.Lstat(targetDir); err == nil {
		targetExists = true
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("checking target before replace: %w", err)
	}

	if !targetExists {
		if err := os.Rename(stagingDir, targetDir); err != nil {
			return fmt.Errorf("installing replacement target: %w", err)
		}
		return nil
	}

	backup, err := os.MkdirTemp(parent, ".coolrestore-backup-")
	if err != nil {
		return fmt.Errorf("creating replace backup name: %w", err)
	}
	if err := os.Remove(backup); err != nil {
		return fmt.Errorf("preparing replace backup name: %w", err)
	}

	if err := os.Rename(targetDir, backup); err != nil {
		return fmt.Errorf("moving existing target for replace: %w", err)
	}
	if err := os.Rename(stagingDir, targetDir); err != nil {
		rollbackErr := os.Rename(backup, targetDir)
		if rollbackErr != nil {
			return fmt.Errorf("installing replacement target: %w; rollback failed: %v", err, rollbackErr)
		}
		return fmt.Errorf("installing replacement target: %w", err)
	}
	if err := os.RemoveAll(backup); err != nil {
		return fmt.Errorf("removing replace backup: %w", err)
	}
	return nil
}
