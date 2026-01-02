package logmsg

import (
	"fmt"
	"os"
)

func Error(message string, err error) {
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "x %s: %s\n", message, err.Error())
		return
	}

	_, _ = fmt.Fprintf(os.Stderr, "x %s\n", message)
}

func Info(message string) {
	_, _ = fmt.Fprintf(os.Stdout, "* "+message+"\n")
}
