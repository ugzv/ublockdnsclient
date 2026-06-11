package service

import (
	"strings"
	"testing"
)

const nextdnsUnitTemplate = `[Unit]
Description=desc

[Service]
StartLimitInterval=5
StartLimitBurst=10
ExecStart=/usr/local/bin/ublockdns run
RestartSec=120
LimitMEMLOCK=infinity

[Install]
WantedBy=multi-user.target
`

func TestPatchSystemdUnitAddsRestartPolicy(t *testing.T) {
	patched, changed := patchSystemdUnit(nextdnsUnitTemplate)
	if !changed {
		t.Fatal("expected the library unit to be patched")
	}
	if !strings.Contains(patched, "Restart=always\nRestartSec=2\n") {
		t.Fatalf("missing restart policy:\n%s", patched)
	}
	if strings.Contains(patched, "RestartSec=120") {
		t.Fatal("stale RestartSec must be replaced")
	}

	again, changed := patchSystemdUnit(patched)
	if changed || again != patched {
		t.Fatal("patch must be idempotent")
	}
}

func TestPatchSystemdUnitLeavesUnknownLayoutsAlone(t *testing.T) {
	custom := "[Service]\nRestart=on-failure\nRestartSec=120\n"
	if _, changed := patchSystemdUnit(custom); changed {
		t.Fatal("units with an explicit Restart= must not be touched")
	}
	if _, changed := patchSystemdUnit("[Service]\nExecStart=/bin/x\n"); changed {
		t.Fatal("units without the known RestartSec line must not be touched")
	}
}
