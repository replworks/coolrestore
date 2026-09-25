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
	return applyReplace(stagingDir, targetDir, plan, replaceFailureNone)
}

type replaceFailurePoint uint8

const (
	replaceFailureNone replaceFailurePoint = iota
	replaceFailureAfterTargetMove
	replaceFailureAfterStagingMove
)

// applyReplace keeps failure injection private to this package so tests can
// exercise interruption points without adding an operational switch.
func applyReplace(stagingDir, targetDir string, plan Plan, failure replaceFailurePoint) error {
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
	if failure == replaceFailureAfterTargetMove {
		if err := os.Rename(backup, targetDir); err != nil {
			return fmt.Errorf("replace interruption: %w; rollback failed: %v", os.ErrInvalid, err)
		}
		return fmt.Errorf("replace interrupted after moving existing target")
	}
	if err := os.Rename(stagingDir, targetDir); err != nil {
		rollbackErr := rollbackReplaceTarget(backup, targetDir)
		if rollbackErr != nil {
			return fmt.Errorf("installing replacement target: %w; rollback failed: %v", err, rollbackErr)
		}
		return fmt.Errorf("installing replacement target: %w", err)
	}
	if failure == replaceFailureAfterStagingMove {
		if err := rollbackReplaceTarget(backup, targetDir); err != nil {
			return fmt.Errorf("replace interruption: %w; rollback failed: %v", os.ErrInvalid, err)
		}
		return fmt.Errorf("replace interrupted after installing staging")
	}
	if err := os.RemoveAll(backup); err != nil {
		if rollbackErr := rollbackReplaceTarget(backup, targetDir); rollbackErr != nil {
			return fmt.Errorf("removing replace backup: %w; rollback failed: %v", err, rollbackErr)
		}
		return fmt.Errorf("removing replace backup: %w", err)
	}
	return nil
}

func rollbackReplaceTarget(backup, target string) error {
	if err := os.RemoveAll(target); err != nil {
		return err
	}
	return os.Rename(backup, target)
}
