package gggmode

import "testing"

func TestVersion(t *testing.T) {
	if Version == "" {
		t.Fatal("Version 不能为空")
	}
}
