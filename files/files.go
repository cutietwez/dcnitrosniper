package files

import (
	"bufio"
	"errors"
	"io"
	"os"
	"sniper/logger"
	"strings"
	"sync"
)

func WorkingDirectory() (dir string, err error) {
	/*ex, err := os.Executable()
	if err != nil {
		return "", err
	}

	exPath := filepath.Dir(ex)
	return exPath, nil*/

	ex, err := os.Getwd()
	if err != nil {
		return "", err
	}

	return ex, nil
}

// fixed: "bufio.Scanner: token too long"
// https://stackoverflow.com/questions/8757389/reading-a-file-line-by-line-in-go
func ReadLines(filePath string) ([]string, error) {
	f, err := os.Open(filePath)
	if errors.Is(err, io.EOF) {
		err = nil
	}
	if err != nil {
		return nil, err
	}

	defer f.Close()

	r := bufio.NewReader(f)
	bytes, lines := []byte{}, []string{}

	for {
		line, isPrefix, err := r.ReadLine()
		if err != nil {
			break
		}

		bytes = append(bytes, line...)
		if !isPrefix {
			str := strings.TrimSpace(string(bytes))

			if len(str) > 0 {
				lines = append(lines, str)
				bytes = []byte{}
			}
		}
	}

	if len(bytes) > 0 {
		lines = append(lines, string(bytes))
	}

	return lines, nil
}

func CreateFileIfNotExists(filePath string) {
	// check if file exists
	var _, err = os.Stat(filePath)

	// create file if not exists
	if os.IsNotExist(err) {
		var file, err = os.Create(filePath)
		if err != nil {
			return
		}
		defer file.Close()
	}
}

func AppendFile(filePath string, Content string) {
	File, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		logger.Error("Failed to append to file", logger.FieldString("path", filePath), logger.FieldAny("error", err))
		return
	}

	defer File.Close()

	_, err = File.WriteString(Content + "\n")
	if err != nil {
		logger.Error("Failed to append to file", logger.FieldString("path", filePath), logger.FieldAny("error", err))
		return
	}
}

func OverwriteFile(filePath string, Content string) {
	File, err := os.Create(filePath)
	if err != nil {
		logger.Error("Failed to overwrite file", logger.FieldString("path", filePath), logger.FieldAny("error", err))
		return
	}

	defer File.Close()

	_, err = File.WriteString(Content)
	if err != nil {
		logger.Error("Failed to append to file", logger.FieldString("path", filePath), logger.FieldAny("error", err))
		return
	}
}

type FileHandle struct {
	init     bool
	file     *os.File
	filePath string
	Mutex    sync.Mutex
}

func (handle *FileHandle) Init(filePath string) (err error) {
	if handle.init {
		err = nil
		return
	}

	handle.file, err = os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}

	handle.filePath = filePath
	handle.init = true
	return
}

func (handle *FileHandle) AppendFile(Content string) {
	handle.Mutex.Lock()
	defer handle.Mutex.Unlock()

	_, err := handle.file.WriteString(Content + "\n")
	if err != nil {
		logger.Error("Failed to append to file", logger.FieldString("path", handle.filePath), logger.FieldAny("error", err))
		return
	}
}

func (handle *FileHandle) Close() {
	if !handle.init {
		return
	}

	_ = handle.file.Close()
}
