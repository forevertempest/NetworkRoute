//go:build windows

package console

import (
	"golang.org/x/sys/windows"
	"os"
)

func terminal() (bool, func()) {
	out, in := windows.Handle(os.Stdout.Fd()), windows.Handle(os.Stdin.Fd())
	var oldMode, inputMode uint32
	if windows.GetConsoleMode(out, &oldMode) != nil || windows.GetConsoleMode(in, &inputMode) != nil {
		return false, func() {}
	}
	dll := windows.NewLazySystemDLL("kernel32.dll")
	getOut, getIn := dll.NewProc("GetConsoleOutputCP"), dll.NewProc("GetConsoleCP")
	setOut, setIn := dll.NewProc("SetConsoleOutputCP"), dll.NewProc("SetConsoleCP")
	oldOut, _, _ := getOut.Call()
	oldIn, _, _ := getIn.Call()
	setOut.Call(65001)
	setIn.Call(65001)
	ansi := windows.SetConsoleMode(out, oldMode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING) == nil
	return ansi, func() { windows.SetConsoleMode(out, oldMode); setOut.Call(oldOut); setIn.Call(oldIn) }
}
