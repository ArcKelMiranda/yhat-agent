package main

import (
	"os"
	"testing"
	"time"

	"github.com/ArcKelMiranda/yhat-agent/internal/bandeja"
)

func withArgs(t *testing.T, args ...string) {
	t.Helper()
	old := os.Args
	os.Args = append([]string{"yhat-agent", "bandeja"}, args...)
	t.Cleanup(func() { os.Args = old })
}

func TestIdleFlag(t *testing.T) {
	cases := []struct {
		args    []string
		want    time.Duration
		wantErr bool
	}{
		{nil, bandeja.DefaultIdle, false},
		{[]string{"--idle", "30m"}, 30 * time.Minute, false},
		{[]string{"--idle=90s"}, 90 * time.Second, false},
		{[]string{"--no-bandeja", "--idle", "1h"}, time.Hour, false},
		{[]string{"--idle", "abc"}, 0, true},
		{[]string{"--idle", "0"}, 0, true},
		{[]string{"--idle", "-5m"}, 0, true},
		{[]string{"--idle"}, 0, true},
	}
	for _, c := range cases {
		withArgs(t, c.args...)
		got, err := idleFlag()
		if (err != nil) != c.wantErr || (!c.wantErr && got != c.want) {
			t.Errorf("args %v: got %v, err %v; want %v, err=%v", c.args, got, err, c.want, c.wantErr)
		}
	}
}
