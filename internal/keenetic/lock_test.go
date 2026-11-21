// SPDX-License-Identifier: GPL-2.0-or-later
package keenetic

import "testing"

func TestSingleStateOwner(t *testing.T) {
	dir := t.TempDir()
	first, err := LockState(dir)
	if err != nil {
		t.Fatal(err)
	}
	if second, err := LockState(dir); err == nil {
		second.Close()
		t.Fatal("second owner acquired state")
	}
	first.Close()
	third, err := LockState(dir)
	if err != nil {
		t.Fatal(err)
	}
	third.Close()
}
