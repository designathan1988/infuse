package core

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
)

// msgError is an error that is a message of the catalog. Error() is the model's version (English), or
// the user's language when the error is answered to the screen (UserError). Whoever shows it can take
// the message itself (MsgOf) and write it in any language — nothing parses its text.
type msgError struct {
	m    Msg
	lang string
}

func (e *msgError) Error() string {
	if e.lang != "" {
		return e.m.Render(e.lang)
	}
	return e.m.String()
}

// ErrMsg is an error the model reads (a tool's refusal that travels as an error).
func ErrMsg(m Msg) error { return &msgError{m: m} }

// UserError is an error answered to the screen, already in the user's language.
func UserError(lang string, m Msg) error { return &msgError{m: m, lang: lang} }

// MsgOf is the message an error carries, when it is one of infuse's.
func MsgOf(err error) (Msg, bool) {
	var me *msgError
	if errors.As(err, &me) {
		return me.m, true
	}
	return Msg{}, false
}

// ErrArg is an error as a message argument: infuse's own message (written in the reader's language) or
// the error's text (an operating system's or a server's, kept as it is).
func ErrArg(err error) any {
	if err == nil {
		return ""
	}
	if m, ok := MsgOf(err); ok {
		return m
	}
	return err.Error()
}

// ExitCode is the status a failed child process exited with (127 when there is no status).
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	fmt.Fprintln(os.Stderr, "infuse:", err)
	return 127
}
