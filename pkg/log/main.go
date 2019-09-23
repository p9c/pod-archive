package log

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	gt "github.com/buger/goterm"
)

var (
	colorOff     = "\033[0m"
	colorRed     = "\033[0;31m"
	colorRedB    = "\033[0;31;1m"
	colorGreen   = "\033[0;32m"
	colorGreenB  = "\033[0;32;1m"
	colorOrange  = "\033[0;33m"
	colorOrangeB = "\033[0;33;1m"
	colorBlue    = "\033[0;34m"
	colorBlueB   = "\033[0;34;1m"
	colorPurple  = "\033[0;35m"
	colorPurpleB = "\033[0;35;1m"
	colorCyan    = "\033[0;36m"
	colorCyanB   = "\033[0;36;1m"
	colorGray    = "\033[0;37m"
	colorGrayB   = "\033[0;37;1m"
	styleBold    = "\033[1m"
)

var StartupTime = time.Now()

type PrintlnFunc func(a ...interface{})
type PrintfFunc func(format string, a ...interface{})
type Closure func(func() string)

const (
	Off   = "off"
	Fatal = "fatal"
	Error = "error"
	Warn  = "warn"
	Info  = "info"
	Debug = "debug"
	Trace = "trace"
)

var Levels = []string{
	Off, Fatal, Error, Warn, Info, Debug, Trace,
}

// Logger is a struct containing all the functions with nice handy names
type Logger struct {
	Fatal         PrintlnFunc
	Error         PrintlnFunc
	Warn          PrintlnFunc
	Info          PrintlnFunc
	Debug         PrintlnFunc
	Trace         PrintlnFunc
	Fatalf        PrintfFunc
	Errorf        PrintfFunc
	Warnf         PrintfFunc
	Infof         PrintfFunc
	Debugf        PrintfFunc
	Tracef        PrintfFunc
	Fatalc        Closure
	Errorc        Closure
	Warnc         Closure
	Infoc         Closure
	Debugc        Closure
	Tracec        Closure
	LogFileHandle *os.File
	Color         bool
}

// Entry is a log entry to be printed as json to the log file
type Entry struct {
	Time         time.Time
	Level        string
	CodeLocation string
	Text         string
}

func Empty() *Logger {
	return &Logger{
		Fatal:  NoPrintln(),
		Error:  NoPrintln(),
		Warn:   NoPrintln(),
		Info:   NoPrintln(),
		Debug:  NoPrintln(),
		Trace:  NoPrintln(),
		Fatalf: NoPrintf(),
		Errorf: NoPrintf(),
		Warnf:  NoPrintf(),
		Infof:  NoPrintf(),
		Debugf: NoPrintf(),
		Tracef: NoPrintf(),
		Fatalc: NoClosure(),
		Errorc: NoClosure(),
		Warnc:  NoClosure(),
		Infoc:  NoClosure(),
		Debugc: NoClosure(),
		Tracec: NoClosure(),
	}

}

// sanitizeLoglevel accepts a string and returns a
// default if the input is not in the Levels slice
func sanitizeLoglevel(level string) string {
	found := false
	for i := range Levels {
		if level == Levels[i] {
			found = true
			break
		}
	}
	if !found {
		level = "info"
	}
	return level
}

// SetLogPaths sets a file path to write logs
func (l *Logger) SetLogPaths(logPath, logFileName string) {
	const timeFormat = "2006-01-02_15-04-05"
	path := filepath.Join(logFileName, logPath)
	var logFileHandle *os.File
	if FileExists(path) {
		err := os.Rename(path, filepath.Join(logPath,
			time.Now().Format(timeFormat)+".json"))
		if err != nil {
			fmt.Println("error rotating log", err)
			return
		}
	}
	logFileHandle, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0755)
	if err != nil {
		fmt.Println("error opening log file", logFileName)
	}
	l.LogFileHandle = logFileHandle
	_, _ = fmt.Fprintln(logFileHandle, "{")
}

// SetLevel enables or disables the various print functions
func (l *Logger) SetLevel(level string, color bool) {
	_, loc, line, _ := runtime.Caller(1)
	files := strings.Split(loc, "github.com/parallelcointeam/parallelcoin/")
	codeLoc := fmt.Sprint(files[1], ":", justifyLineNumber(line))
	fmt.Println("setting level to", level, codeLoc)
	*l = *Empty()
	var fallen bool
	switch {
	case level == Trace || fallen:
		// fmt.Println("loading Trace printers")
		l.Trace = Println("TRC", color, l.LogFileHandle)
		l.Tracef = Printf("TRC", color, l.LogFileHandle)
		l.Tracec = Printc("TRC", color, l.LogFileHandle)
		fallen = true
		fallthrough
	case level == Debug || fallen:
		// fmt.Println("loading Debug printers")
		l.Debug = Println("DBG", color, l.LogFileHandle)
		l.Debugf = Printf("DBG", color, l.LogFileHandle)
		l.Debugc = Printc("DBG", color, l.LogFileHandle)
		fallen = true
		fallthrough
	case level == Info || fallen:
		// fmt.Println("loading Info printers")
		l.Info = Println("INF", color, l.LogFileHandle)
		l.Infof = Printf("INF", color, l.LogFileHandle)
		l.Infoc = Printc("INF", color, l.LogFileHandle)
		fallen = true
		fallthrough
	case level == Warn || fallen:
		// fmt.Println("loading Warn printers")
		l.Warn = Println("WRN", color, l.LogFileHandle)
		l.Warnf = Printf("WRN", color, l.LogFileHandle)
		l.Warnc = Printc("WRN", color, l.LogFileHandle)
		fallen = true
		fallthrough
	case level == Error || fallen:
		// fmt.Println("loading Error printers")
		l.Error = Println("ERR", color, l.LogFileHandle)
		l.Errorf = Printf("ERR", color, l.LogFileHandle)
		l.Errorc = Printc("ERR", color, l.LogFileHandle)
		fallen = true
		fallthrough
	case level == Fatal:
		// fmt.Println("loading Fatal printers")
		l.Fatal = Println("FTL", color, l.LogFileHandle)
		l.Fatalf = Printf("FTL", color, l.LogFileHandle)
		l.Fatalc = Printc("FTL", color, l.LogFileHandle)
		fallen = true
	}
}

var NoPrintln = func() func(_ ...interface{}) {
	return func(_ ...interface{}) {
	}
}
var NoPrintf = func() func(_ string, _ ...interface{}) {
	return func(_ string, _ ...interface{}) {
	}
}
var NoClosure = func() func(_ func() string) {
	return func(_ func() string) {
	}
}

func trimReturn(s string) string {
	if s[len(s)-1] == '\n' {
		return s[:len(s)-1]
	}
	return s
}

func justifyLineNumber(n int) string {
	s := fmt.Sprint(n)
	switch len(s) {
	case 1:
		s += "    "
	case 2:
		s += "   "
	case 3:
		s += "  "
	case 4:
		s += " "
	}
	return s
}

// RightJustify takes a string and right justifies it by a width or crops it
func rightJustify(s string, w int) string {
	sw := len(s)
	diff := w - sw
	if diff > 0 {
		s = strings.Repeat(" ", diff) + s
	} else if diff < 0 {
		s = s[:w]
	}
	return s
}

func composit(text, level string, color bool) string {
	terminalWidth := gt.Width()
	_, loc, iline, _ := runtime.Caller(3)
	line := fmt.Sprint(iline)
	files := strings.Split(loc, "github.com/parallelcointeam/parallelcoin/")
	file := files[1]
	since := fmt.Sprint(time.Now().Sub(StartupTime) / time.
		Second * time.Second)
	if terminalWidth > 144 {
		since = fmt.Sprint(time.Now())[:24]
	}
	levelLen := len(level) + 1
	sinceLen := len(since) + 1
	textLen := len(text) + 1
	fileLen := len(file) + 1
	lineLen := len(line) + 1
	final := "" // fmt.Sprintf("%s %s %s %s:%s", level, since, text, file, line)
	if levelLen+sinceLen+textLen+fileLen+lineLen > terminalWidth {
		lines := strings.Split(text, "\n")
		// log text is multiline
		line1len := terminalWidth - levelLen - sinceLen - fileLen - lineLen
		restLen := terminalWidth - levelLen - sinceLen
		if len(lines) > 1 {
			final = fmt.Sprintf("%s %s %s %s:%s", level, since,
				strings.Repeat(" ",
					terminalWidth-levelLen-sinceLen-fileLen-lineLen),
				file, line)
			for i := range lines {
				maxPreformatted := 68 - levelLen - sinceLen
				ll := lines[i]
				var slices []string
				for len(ll) > maxPreformatted {
					// if lopping the last space-bound block drops the line
					// under terminalWidth  do that instead of cutting for
					// the hex dumps
					cs := strings.Split(ll, " ")
					lenLast := len(cs[len(cs)-1])
					if len(ll)-lenLast <= maxPreformatted {
						final += ll[:len(ll)-lenLast]+"\n"
						final += cs[len(cs)-1]+"\n"
					} else {
						slices = append(slices, ll[:maxPreformatted])
						ll = ll[maxPreformatted:]
					}
				}
				slices = append(slices, ll)
				for j := range slices {
					if j > 0 {
						final += "\n" + strings.Repeat(" ",
							terminalWidth-len(slices[j])-2) + "->" + slices[j]
					} else {
						final += "\n" + strings.Repeat(" ", levelLen+sinceLen) + slices[j]
					}
				}
				//
				// final += "\n" + strings.Repeat(" ",
				// 	levelLen+sinceLen) + lines[i]
			}
		} else {
			// log text is a long line
			spaced := strings.Split(text, " ")
			var rest bool
			curLineLen := 0
			final += fmt.Sprintf("%s %s ", level, since)
			var i int
			for i = range spaced {
				if i > 0 {
					curLineLen += len(spaced[i-1]) + 1
					if !rest {
						if curLineLen >= line1len {
							rest = true
							spacers := terminalWidth - levelLen - sinceLen -
								fileLen - lineLen - curLineLen + len(spaced[i-1]) + 1
							final += strings.Repeat(".", spacers)
							final += fmt.Sprintf(" %s:%s\n",
								file, line)
							final += strings.Repeat(" ", levelLen+sinceLen)
							final += spaced[i-1] + " "
							curLineLen = len(spaced[i-1]) + 1
						} else {
							final += spaced[i-1] + " "
						}
					} else {
						if curLineLen >= restLen-1 {
							final += "\n" + strings.Repeat(" ",
								levelLen+sinceLen)
							final += spaced[i-1] + "."
							curLineLen = len(spaced[i-1]) + 1
						} else {
							final += spaced[i-1] + " "
						}
					}
				}
			}
			curLineLen += len(spaced[i])
			if !rest {
				if curLineLen >= line1len {
					final += fmt.Sprintf("%s %s:%s\n",
						strings.Repeat(".",
							len(spaced[i])+line1len-curLineLen),
						file, line)
					final += strings.Repeat(" ", levelLen+sinceLen)
					final += spaced[i] // + "\n"
				} else {
					final += fmt.Sprintf("%s %s %s:%s\n",
						spaced[i],
						strings.Repeat(".",
							terminalWidth-curLineLen-fileLen-lineLen),
						file, line)
				}
			} else {
				if curLineLen >= restLen {
					final += "\n" + strings.Repeat(" ", levelLen+sinceLen)
				}
				final += spaced[i] // + "\n"
			}
		}
	} else {
		final = fmt.Sprintf("%s %s %s %s %s:%s", level, since, text,
			strings.Repeat(".",
				terminalWidth-levelLen-sinceLen-textLen-fileLen-lineLen),
			file, line)
	}
	return final
}

// Println prints a log entry like Println
func Println(level string, color bool, fh *os.File) func(a ...interface{}) {
	return func(a ...interface{}) {
		text := trimReturn(fmt.Sprintln(a...))
		fmt.Println(composit(text, level, color))
		if fh != nil {
			_, loc, line, _ := runtime.Caller(2)
			out := Entry{time.Now(), level, fmt.Sprint(loc, ":", line), text}
			j, err := json.Marshal(out)
			if err != nil {
				fmt.Println("logging error:", err)
			}
			_, _ = fmt.Fprint(fh, string(j)+",")
		}
	}
}

// Printf prints a log entry with formatting
func Printf(level string, color bool, fh *os.File) func(format string,
	a ...interface{}) {
	return func(format string, a ...interface{}) {
		text := fmt.Sprintf(format, a...)
		fmt.Println(composit(text, level, color))
		if fh != nil {
			_, loc, line, _ := runtime.Caller(2)
			out := Entry{time.Now(), level, fmt.Sprint(loc, ":", line), text}
			j, err := json.Marshal(out)
			if err != nil {
				fmt.Println("logging error:", err)
			}
			_, _ = fmt.Fprint(fh, string(j)+",")
		}
	}
}

// Printc prints from a closure returning a string
func Printc(level string, color bool, fh *os.File) func(fn func() string) {
	return func(fn func() string) {
		// level = strings.ToUpper(string(level[0]))
		t := fn()
		text := trimReturn(t)
		fmt.Println(composit(text, level, color))
		if fh != nil {
			_, loc, line, _ := runtime.Caller(2)
			out := Entry{time.Now(), level, fmt.Sprint(loc, ":", line), text}
			j, err := json.Marshal(out)
			if err != nil {
				fmt.Println("logging error:", err)
			}
			_, _ = fmt.Fprint(fh, string(j)+",")
		}
	}
}

// FileExists reports whether the named file or directory exists.
func FileExists(filePath string) bool {
	_, err := os.Stat(filePath)
	return err == nil
}

// DirectionString is a helper function that returns a string that represents the direction of a connection (inbound or outbound).
func DirectionString(inbound bool) string {
	if inbound {
		return "inbound"
	}
	return "outbound"
}

// PickNoun returns the singular or plural form of a noun depending
// on the count n.
func PickNoun(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}