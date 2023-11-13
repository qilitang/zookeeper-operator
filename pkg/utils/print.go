package utils

import (
	"fmt"
	"io"
	"strconv"
)

// Iline writes indented line with \n into provided io.Writer
func Iline(w io.Writer, indent int, format string, a ...interface{}) {
	if indent > 0 {
		// Prepare indentation template %16s
		template := "%" + strconv.Itoa(indent) + "s"
		Fprintf(w, template, " ")
	}
	Fprintf(w, format, a...)
	Fprintf(w, "\n")
}

// Fprintf suppresses warning for unused returns of fmt.Fprintf()
func Fprintf(w io.Writer, format string, a ...interface{}) {
	_, _ = fmt.Fprintf(w, format, a...)
}
