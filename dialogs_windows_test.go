//go:build windows

package main

import "testing"

func TestDialogOptions(t *testing.T) {
	f := dialogOptions(true)
	if f&fosPickFolders == 0 || f&fosAllowMultiselect != 0 {
		t.Fatalf("folder picker flags: %x", f)
	}
	g := dialogOptions(false)
	if g&fosPickFolders != 0 || g&fosAllowMultiselect == 0 || g&fosFileMustExist == 0 {
		t.Fatalf("file picker flags: %x", g)
	}
}

func TestOSDialogsIsNative(t *testing.T) {
	if _, ok := osDialogs(nil).(winDialogs); !ok {
		t.Fatal("windows must use IFileOpenDialog")
	}
}
