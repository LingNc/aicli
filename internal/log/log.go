package log

import (
	"fmt"
	"os"
)

var debug bool

func SetDebug(enabled bool) {
	debug = enabled
}

func IsDebug() bool {
	return debug
}

func Debug(format string, args ...interface{}) {
	if debug {
		fmt.Fprintf(os.Stderr, "\r[DEBUG] "+format+"\r\n", args...)
	}
}

func Info(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "\r"+format+"\r\n", args...)
}
