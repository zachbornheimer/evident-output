// Fixture for adopt's mutation-facade detection (ZYS-1019). This mirrors
// homelab's launchdfacade/dockerfacade/filesystemfacade shape: a package
// named *facade whose CLI type has no io.Writer field at all — every
// method shells out or writes to disk instead of wrapping a writer — but
// still deserves a "migrate the facade" finding the way an output facade
// does, since Bootstrap/Up/Write* are the mutations the adoption ladder's
// effects rung cares about. adopt's io.Writer-field facade detector cannot
// see this on its own.
package mutationfacade

import "os/exec"

// CLI is the real, launchctl/docker-cli-backed facade — no io.Writer field,
// every method is a side-effecting call named for the mutation it performs.
type CLI struct {
	Bin string
}

// Bootstrap loads a launchd job, the real mutation launchdfacade.CLI performs.
func (c CLI) Bootstrap(uid, plistPath string) error {
	return exec.Command(c.Bin, "bootstrap", uid, plistPath).Run()
}

// Up brings a compose service up, the real mutation dockerfacade.CLI performs.
func (c CLI) Up(composeFile, service string) error {
	return exec.Command(c.Bin, "up", "-f", composeFile, service).Run()
}

// WriteConfig writes a config file to disk, the real mutation
// filesystemfacade.CLI performs.
func (c CLI) WriteConfig(path string, data []byte) error {
	return exec.Command(c.Bin, "write", path).Run()
}
