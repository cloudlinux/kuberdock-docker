// +build !windows

package runconfig

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/docker/engine-api/types/container"
)

// TODO Windows: This will need addressing for a Windows daemon.
func TestNetworkModeTest(t *testing.T) {
	networkModes := map[container.NetworkMode][]bool{
		// private, bridge, host, container, none, default
		"":                         {true, false, false, false, false, false},
		"something:weird":          {true, false, false, false, false, false},
		"bridge":                   {true, true, false, false, false, false},
		DefaultDaemonNetworkMode(): {true, true, false, false, false, false},
		"host":           {false, false, true, false, false, false},
		"container:name": {false, false, false, true, false, false},
		"none":           {true, false, false, false, true, false},
		"default":        {true, false, false, false, false, true},
	}
	networkModeNames := map[container.NetworkMode]string{
		"":                         "",
		"something:weird":          "something:weird",
		"bridge":                   "bridge",
		DefaultDaemonNetworkMode(): "bridge",
		"host":           "host",
		"container:name": "container",
		"none":           "none",
		"default":        "default",
	}
	for networkMode, state := range networkModes {
		if networkMode.IsPrivate() != state[0] {
			t.Fatalf("NetworkMode.IsPrivate for %v should have been %v but was %v", networkMode, state[0], networkMode.IsPrivate())
		}
		if networkMode.IsBridge() != state[1] {
			t.Fatalf("NetworkMode.IsBridge for %v should have been %v but was %v", networkMode, state[1], networkMode.IsBridge())
		}
		if networkMode.IsHost() != state[2] {
			t.Fatalf("NetworkMode.IsHost for %v should have been %v but was %v", networkMode, state[2], networkMode.IsHost())
		}
		if networkMode.IsContainer() != state[3] {
			t.Fatalf("NetworkMode.IsContainer for %v should have been %v but was %v", networkMode, state[3], networkMode.IsContainer())
		}
		if networkMode.IsNone() != state[4] {
			t.Fatalf("NetworkMode.IsNone for %v should have been %v but was %v", networkMode, state[4], networkMode.IsNone())
		}
		if networkMode.IsDefault() != state[5] {
			t.Fatalf("NetworkMode.IsDefault for %v should have been %v but was %v", networkMode, state[5], networkMode.IsDefault())
		}
		if networkMode.NetworkName() != networkModeNames[networkMode] {
			t.Fatalf("Expected name %v, got %v", networkModeNames[networkMode], networkMode.NetworkName())
		}
	}
}

func TestIpcModeTest(t *testing.T) {
	ipcModes := map[container.IpcMode][]bool{
		// private, host, container, valid
		"":                         {true, false, false, true},
		"something:weird":          {true, false, false, false},
		":weird":                   {true, false, false, true},
		"host":                     {false, true, false, true},
		"container:name":           {false, false, true, true},
		"container:name:something": {false, false, true, false},
		"container:":               {false, false, true, false},
	}
	for ipcMode, state := range ipcModes {
		if ipcMode.IsPrivate() != state[0] {
			t.Fatalf("IpcMode.IsPrivate for %v should have been %v but was %v", ipcMode, state[0], ipcMode.IsPrivate())
		}
		if ipcMode.IsHost() != state[1] {
			t.Fatalf("IpcMode.IsHost for %v should have been %v but was %v", ipcMode, state[1], ipcMode.IsHost())
		}
		if ipcMode.IsContainer() != state[2] {
			t.Fatalf("IpcMode.IsContainer for %v should have been %v but was %v", ipcMode, state[2], ipcMode.IsContainer())
		}
		if ipcMode.Valid() != state[3] {
			t.Fatalf("IpcMode.Valid for %v should have been %v but was %v", ipcMode, state[3], ipcMode.Valid())
		}
	}
	containerIpcModes := map[container.IpcMode]string{
		"":                      "",
		"something":             "",
		"something:weird":       "weird",
		"container":             "",
		"container:":            "",
		"container:name":        "name",
		"container:name1:name2": "name1:name2",
	}
	for ipcMode, container := range containerIpcModes {
		if ipcMode.Container() != container {
			t.Fatalf("Expected %v for %v but was %v", container, ipcMode, ipcMode.Container())
		}
	}
}

func TestUTSModeTest(t *testing.T) {
	utsModes := map[container.UTSMode][]bool{
		// private, host, valid
		"":                {true, false, true},
		"something:weird": {true, false, false},
		"host":            {false, true, true},
		"host:name":       {true, false, true},
	}
	for utsMode, state := range utsModes {
		if utsMode.IsPrivate() != state[0] {
			t.Fatalf("UtsMode.IsPrivate for %v should have been %v but was %v", utsMode, state[0], utsMode.IsPrivate())
		}
		if utsMode.IsHost() != state[1] {
			t.Fatalf("UtsMode.IsHost for %v should have been %v but was %v", utsMode, state[1], utsMode.IsHost())
		}
		if utsMode.Valid() != state[2] {
			t.Fatalf("UtsMode.Valid for %v should have been %v but was %v", utsMode, state[2], utsMode.Valid())
		}
	}
}

func TestPidModeTest(t *testing.T) {
	pidModes := map[container.PidMode][]bool{
		// private, host, valid
		"":                {true, false, true},
		"something:weird": {true, false, false},
		"host":            {false, true, true},
		"host:name":       {true, false, true},
	}
	for pidMode, state := range pidModes {
		if pidMode.IsPrivate() != state[0] {
			t.Fatalf("PidMode.IsPrivate for %v should have been %v but was %v", pidMode, state[0], pidMode.IsPrivate())
		}
		if pidMode.IsHost() != state[1] {
			t.Fatalf("PidMode.IsHost for %v should have been %v but was %v", pidMode, state[1], pidMode.IsHost())
		}
		if pidMode.Valid() != state[2] {
			t.Fatalf("PidMode.Valid for %v should have been %v but was %v", pidMode, state[2], pidMode.Valid())
		}
	}
}

func TestRestartPolicy(t *testing.T) {
	restartPolicies := map[container.RestartPolicy][]bool{
		// none, always, failure
		container.RestartPolicy{}:                {false, false, false},
		container.RestartPolicy{"something", 0}:  {false, false, false},
		container.RestartPolicy{"no", 0}:         {true, false, false},
		container.RestartPolicy{"always", 0}:     {false, true, false},
		container.RestartPolicy{"on-failure", 0}: {false, false, true},
	}
	for restartPolicy, state := range restartPolicies {
		if restartPolicy.IsNone() != state[0] {
			t.Fatalf("RestartPolicy.IsNone for %v should have been %v but was %v", restartPolicy, state[0], restartPolicy.IsNone())
		}
		if restartPolicy.IsAlways() != state[1] {
			t.Fatalf("RestartPolicy.IsAlways for %v should have been %v but was %v", restartPolicy, state[1], restartPolicy.IsAlways())
		}
		if restartPolicy.IsOnFailure() != state[2] {
			t.Fatalf("RestartPolicy.IsOnFailure for %v should have been %v but was %v", restartPolicy, state[2], restartPolicy.IsOnFailure())
		}
	}
}
func TestDecodeHostConfig(t *testing.T) {
	fixtures := []struct {
		file string
	}{
		{"fixtures/unix/container_hostconfig_1_14.json"},
		{"fixtures/unix/container_hostconfig_1_19.json"},
	}

	for _, f := range fixtures {
		b, err := ioutil.ReadFile(f.file)
		if err != nil {
			t.Fatal(err)
		}

		c, err := DecodeHostConfig(bytes.NewReader(b))
		if err != nil {
			t.Fatal(fmt.Errorf("Error parsing %s: %v", f, err))
		}

		if c.Privileged != false {
			t.Fatalf("Expected privileged false, found %v\n", c.Privileged)
		}

		if l := len(c.Binds); l != 1 {
			t.Fatalf("Expected 1 bind, found %d\n", l)
		}

		if c.CapAdd.Len() != 1 && c.CapAdd.Slice()[0] != "NET_ADMIN" {
			t.Fatalf("Expected CapAdd NET_ADMIN, got %v", c.CapAdd)
		}

		if c.CapDrop.Len() != 1 && c.CapDrop.Slice()[0] != "NET_ADMIN" {
			t.Fatalf("Expected CapDrop MKNOD, got %v", c.CapDrop)
		}
	}
}
