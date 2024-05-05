package logger

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/inancgumus/screen"
	"golang.org/x/term"
)

var (
	colorsBulletPoint = "131;131;131" //"838383"
	colorsInfo        = "114;127;186" //"727fba"
	colorsFail        = "197;0;0"     //"c50000"
	colorsError       = "255;0;0"     //"ff0000"
	colorsWarn        = "255;253;81"  //"fffd51"
	colorsSuccess     = "127;255;92"  //"7fff5c"
	colorsGeneral     = "220;220;220" //"dcdcdc"
	colorsVarKey      = "131;131;131" //"838383"
	colorsVarValue    = "176;176;176" //"b0b0b0"
	colorsDate        = "61;61;61"    //"3d3d3d"
	colorsSpinner     = "53;96;176"
	loggerMutex       sync.Mutex
)

const (
	// StartSet chars
	startSet = "\x1b["

	// ResetSet close all properties.
	resetSet = "\x1b[0m"

	// Foreground
	fgRGBPfx = "38;2;"

	// Background
	bgRGBPfx = "48;2;"
)

type LogField struct {
	Key   string
	Value string
}

func FieldString(key string, value string) LogField {
	return LogField{
		Key:   key,
		Value: value,
	}
}

func FieldInt(key string, value int) LogField {
	return LogField{
		Key:   key,
		Value: strconv.Itoa(value),
	}
}

func FieldFloat32(key string, value float32) LogField {
	return LogField{
		Key:   key,
		Value: fmt.Sprintf("%f", value),
	}
}

func FieldFloat64(key string, value float64) LogField {
	return LogField{
		Key:   key,
		Value: fmt.Sprintf("%f", value),
	}
}

func FieldAny(key string, value any) LogField {
	return LogField{
		Key:   key,
		Value: fmt.Sprintf("%v", value),
	}
}

// parses if it's a hex to "r;g;b;a"
func parseColorCode(code string) string {
	if code[0] == '#' {
		code = code[1:]
	}

	var r, g, b uint8
	values, err := strconv.ParseUint(code, 16, 32)
	if err != nil {
		return code
	}

	r = uint8(values >> 16)
	g = uint8((values >> 8) & 0xFF)
	b = uint8(values & 0xFF)
	// a = uint8((values >> 24) & 0xff)
	// if a <= 0 {
	// 	a = 255
	// }

	return fmt.Sprintf("%d;%d;%d", int(r), int(g), int(b))
}

func colorStringForeground(code string, str string) string {
	return startSet + fgRGBPfx + parseColorCode(code) + "m" + str + resetSet
}

func colorStringBackground(code string, str string) string {
	return startSet + bgRGBPfx + parseColorCode(code) + "m" + str + resetSet
}

func resolveFields(fields []LogField) string {
	var ret = ""
	if len(fields) > 0 {
		//ret += fmt.Sprintf(" <fg=%s>→</> ", colorsGeneral)
		ret += colorStringForeground(colorsGeneral, " → ")
		for i, v := range fields {
			//ret += fmt.Sprintf("<fg=%s>%s</>=<fg=%s>%s</>", colorsVarKey, v.Key, colorsVarValue, v.Value)
			ret += colorStringForeground(colorsVarKey, v.Key) + "=" + colorStringForeground(colorsVarValue, v.Value)
			if i < len(fields) {
				ret += " "
			}
		}
	}

	return ret
}

func helperLog(logType string, logColor string, description string, fields ...LogField) {
	loggerMutex.Lock()
	fmt.Println(colorStringForeground(colorsDate, time.Now().Format("15:04:05")) + " " + colorStringForeground(logColor, logType) + " " + colorStringForeground(colorsBulletPoint, "●") + " " + colorStringForeground(colorsGeneral, description) + resolveFields(fields) + strings.Repeat(" ", 70))
	loggerMutex.Unlock()
}

func Info(description string, fields ...LogField) {
	helperLog("INFO", colorsInfo, description, fields...)
}

func Warn(description string, fields ...LogField) {
	helperLog("WARN", colorsWarn, description, fields...)
}

func Fail(description string, fields ...LogField) {
	helperLog("FAIL", colorsFail, description, fields...)
}

func Error(description string, fields ...LogField) {
	helperLog("ERROR", colorsError, description, fields...)
}

func Success(description string, fields ...LogField) {
	helperLog("SUCCESS", colorsSuccess, description, fields...)
}

// Show the cursor if it was hidden previously.
// Don't forget to show the cursor at least at the end of your application.
// Otherwise the user might have a terminal with a permanently hidden cursor, until they reopen the terminal.
// https://github.com/atomicgo/cursor/blob/main/cursor.go
func ShowTerminalCursor() {
	fmt.Fprint(os.Stdout, "\x1b[?25h")
}

// Hide the cursor.
// Don't forget to show the cursor at least at the end of your application with Show.
// Otherwise the user might have a terminal with a permanently hidden cursor, until they reopen the terminal.
// https://github.com/atomicgo/cursor/blob/main/cursor.go
func HideTerminalCursor() {
	fmt.Fprintf(os.Stdout, "\x1b[?25l")
}

var spinnerIterator int = 0

/*
Nitro-Sniper Specific only!
This is the spinner thing that basically does the always-on-top stuff in terminal
*/
func CallSpinnerTitle(spinner, text string) {
	spinnerIterator++
	var shouldReturn bool = false

	// did this shit because it would spam the terminal for no reason
	// and it was NOT readable, atleast this way it is readable
	finalChar := "\r"
	terminalW, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err == nil && terminalW > 0 {
		textLen := len("[" + spinner + "] " + text + strings.Repeat(" ", 5))
		if terminalW <= textLen {
			shouldReturn = true
		}
	} else {
		shouldReturn = true
	}

	if shouldReturn {
		finalChar = "\n"
	}

	// we call the spinner every 150ms
	// by doing this, we only call the spinner every minute
	// 1 minute = 60 seconds = 60000 ms
	if spinnerIterator%400 == 0 {
		shouldReturn = false
	}

	// hey, it's invisible, fuck it
	// https://stackoverflow.com/questions/4651012/why-is-the-default-terminal-width-80-characters
	if err == nil && terminalW <= 80 {
		shouldReturn = true

		// let's call this once an hour, who cares
		if spinnerIterator%4000 == 0 {
			shouldReturn = false
		}
	}

	if shouldReturn {
		return
	}

	loggerMutex.Lock()
	fmt.Printf(colorStringForeground(colorsBulletPoint, "[") + colorStringForeground(colorsSpinner, spinner) + colorStringForeground(colorsBulletPoint, "]") + " " + colorStringForeground(colorsGeneral, text) + strings.Repeat(" ", 5) + finalChar)
	loggerMutex.Unlock()
}

func PrintLogo(shouldClear bool) {
	if shouldClear {
		screen.Clear()
		screen.MoveTopLeft()
	}

	fmt.Println(colorStringForeground("a60000", `
			__
			\ \				
			 \ \			
			__\ \			
			\  __\			Twez Sniper
			 \ \			discord.gg/nitrohaven
			__\ \			t.me/nitrohaven
			\  __\			@twezted
			 \ \			donkey kong!
			__\ \
			\  __\
			 \ \
			  \ \
			   \/
	`))
}
