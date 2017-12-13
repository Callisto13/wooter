package wooter

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"code.cloudfoundry.org/lager"
	specs "github.com/opencontainers/runtime-spec/specs-go"
)

const VolumesDir string = "volumes"
const DiffsDir string = "diffs"
const Maximus int = 4294967294

type Cp struct {
	BaseDir string
}

func (c Cp) Unpack(logger lager.Logger, id, parentID string, tar io.Reader) error {
	dest := filepath.Join(c.BaseDir, VolumesDir, id)

	logger.Info("creating-dir", lager.Data{
		"dir": dest,
	})

	if err := os.MkdirAll(dest, 0755); err != nil {
		return err
	}

	parentDir := filepath.Join(c.BaseDir, VolumesDir, parentID)
	if parentID != "" && !isEmptyDir(parentDir) {
		command := fmt.Sprintf("cp -r %s/* %s", filepath.Join(c.BaseDir, VolumesDir, parentID), dest+"/")
		logger.Info("running-command-unplack", lager.Data{
			"command": command,
		})
		cpCmd := exec.Command("sh", "-c", command)
		if out, err := cpCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("%s: %s", string(out), err)
		}
	}

	logger.Info("running-command-unpack", lager.Data{
		"command": fmt.Sprintf("tar -x -C %s", dest),
	})

	tarCmd := exec.Command("tar", "-x", "-C", dest)
	tarCmd.Stdin = tar
	if err := tarCmd.Run(); err != nil {
		return err
	}

	return nil
}

func (c Cp) Bundle(logger lager.Logger, handle string, layerIds []string) (specs.Spec, error) {
	volumeDir := filepath.Join(c.BaseDir, VolumesDir, layerIds[len(layerIds)-1])
	destDir := filepath.Join(c.BaseDir, DiffsDir, handle)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return specs.Spec{}, err
	}

	command := fmt.Sprintf("cp -r %s/* %s", volumeDir, destDir+"/")
	logger.Info("running-command-bundle", lager.Data{
		"command": command,
	})

	cpCmd := exec.Command("sh", "-c", command)
	if out, err := cpCmd.CombinedOutput(); err != nil {
		return specs.Spec{}, fmt.Errorf("%s: %s", string(out), err)
	}

	err := chownToMaximus(destDir)
	if err != nil {
		return specs.Spec{}, err
	}

	return specs.Spec{
		Root: &specs.Root{
			Path: destDir,
		},
	}, nil
}

func (c Cp) Exists(logger lager.Logger, id string) bool {
	_, err := os.Stat(filepath.Join(c.BaseDir, VolumesDir, id))
	return err == nil
}

func chownToMaximus(path string) error {
	return recursiveChown(path, Maximus, Maximus)
}

func recursiveChown(path string, uid, gid int) error {
	return filepath.Walk(path, func(name string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.Mode() == os.ModeSymlink {
			// Do not chown symlinks, we'll be eventually chowning the files they link to instead
			return nil
		}

		return os.Chown(name, uid, gid)
	})
}

func isEmptyDir(name string) bool {
	f, err := os.Open(name)
	if err != nil {
		return true
	}
	defer f.Close()

	_, err = f.Readdirnames(1)
	return err == io.EOF
}
